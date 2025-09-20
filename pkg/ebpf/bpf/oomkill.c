//go:build ignore
// You can generate vmlinux.h to develop this following command in your setup.
// Make sure you keep the original headers as it is.
// `bpftool btf dump file /sys/kernel/btf/vmlinux format c > headers/vmlinux.h`
#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"

char __license[] SEC("license") = "Dual MIT/GPL";

struct event {
    u32 victim_pid;
    u32 trigger_pid;
    char victim_comm[TASK_COMM_LEN];
    char trigger_comm[TASK_COMM_LEN];
    u64 totalpages;
    bool global_oom;
    char names[4][128];  // [0]=leaf, [1]=parent, [2]=grandparent, [3]=great-grandparent]
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} events SEC(".maps");

// Force emitting struct event into the ELF.
const struct event *unused __attribute__((unused));

SEC("kprobe/oom_kill_process")
int BPF_KPROBE(kprobe__oom_kill_process, struct oom_control *oc) {
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e)
        return 0;

    // Always start from a clean slate
    __builtin_memset(e, 0, sizeof(*e));

    // Trigger-side info (never fails)
    e->trigger_pid = bpf_get_current_pid_tgid() >> 32;
    bpf_get_current_comm(&e->trigger_comm, sizeof(e->trigger_comm));
    bpf_probe_read_kernel(&e->totalpages, sizeof(e->totalpages), &oc->totalpages);

    // Victim-side info – check that we really have one
    struct task_struct *victim = BPF_CORE_READ(oc, chosen);   /* CO-RE safe */
    if (victim && (unsigned long)victim != (unsigned long)-1) {
        e->victim_pid = BPF_CORE_READ(victim, pid);
        bpf_core_read_str(&e->victim_comm, sizeof(e->victim_comm),
                          &victim->comm);
    }

    struct mem_cgroup *memcg = BPF_CORE_READ(oc, memcg);
    e->global_oom = (memcg == NULL);

    const char *leaf = NULL;
    struct kernfs_node *kn = NULL;

    if (victim) {
        struct cgroup *dcg = BPF_CORE_READ(victim, cgroups, dfl_cgrp);
        if (dcg)
            kn = BPF_CORE_READ(dcg, kn);
    } else if (memcg) {
        struct cgroup *cg = BPF_CORE_READ(memcg, css.cgroup);
        if (cg)
            kn = BPF_CORE_READ(cg, kn);
    } 

    // Walk up to 4 segments and copy names
    #pragma unroll
    for (int i = 0; i < 4; i++) {
        if (!kn)
            break;
        const char *nm = BPF_CORE_READ(kn, name);
        if (nm)
            bpf_core_read_str(e->names[i], sizeof(e->names[i]), nm);
        kn = BPF_CORE_READ(kn, parent);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}
