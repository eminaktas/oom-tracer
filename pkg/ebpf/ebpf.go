package ebpf

import (
	"fmt"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

type BpfEvent struct {
	bpfEvent
}

type Close func()

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang-20 -type event -target amd64,arm64 -tags linux bpf bpf/oomkill.c -- -I./bpf/headers

func LoadBPF(fn string) (*ringbuf.Reader, Close, error) {
	objs := bpfObjects{}
	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, nil, fmt.Errorf("remove memlock: %w", err)
	}

	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&objs, nil); err != nil {
		return nil, nil, fmt.Errorf("load bpf objects: %w", err)
	}

	// Open a Kprobe at the entry point of the kernel function and attach the
	// pre-compiled program. Each time the kernel function enters, the program
	// will emit an event containing pid, command, and more of the execved task.
	kp, err := link.Kprobe(fn, objs.KprobeOomKillProcess, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("open kprobe on %s: %w", fn, err)
	}

	// Open a ringbuf reader from userspace RINGBUF map described in the
	// eBPF C program.
	rd, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		return nil, nil, fmt.Errorf("open ring buffer: %w", err)
	}

	return rd, func() {
		objs.Close()
		kp.Close()
		rd.Close()
	}, nil
}
