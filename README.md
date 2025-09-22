# OOM Tracer

**Status: experimental** – this project is being iterated on quickly and breaking changes should be expected.

OOM Tracer observes *global* Linux OOM killer events on Kubernetes nodes, maps them back to pods, and annotates the offending workloads so that a descheduler instance can evict them. The intent is to reduce cascading failures that follow severe memory pressure by letting you plug the detection signal into an `AnnotationEvictor` downstream.

## Why Global OOMs Matter

A global OOM is raised when the Linux kernel exhausts system-wide memory and must pick a victim process to reclaim. These events differ from cgroup-local OOMs and are much harder to attribute inside a Kubernetes cluster. See [Chris Siebenmann's overview of when the kernel fires the OOM killer](https://utcc.utoronto.ca/~cks/space/blog/linux/OOMKillerWhen) and [GKE's documentation for System-level OOM kill](https://cloud.google.com/kubernetes-engine/docs/troubleshooting/oom-events#system-oom-kill) for deeper background on the signal we rely on.

By tracing the kernel's `oom_kill_process` path with eBPF we can make each global OOM observable, attach metadata about the victim, and hand that context to higher-level automation.

## How It Works

1. An eBPF program attached to the kernel OOM tracepoint writes kill events into a ring buffer.
2. The Go agent reads those events, normalises them, and attempts to match the victim back to a Kubernetes pod running on the same node.
3. When a pod match is found, the agent annotates the pod with a user-provided key/value pair.
4. The Kubernetes descheduler, extended with the custom `AnnotationEvictor`, watches for that annotation and issues an eviction.

When annotations are disabled (empty flag) the tracer runs in an observation-only mode, which is useful while validating kernel compatibility.

## Relationship With Descheduler

OOM Tracer is designed as a signal producer. Pair it with the [Kubernetes descheduler](https://github.com/kubernetes-sigs/descheduler) configured to run the out-of-tree `AnnotationEvictor`. That evictor interprets the pod annotation written by the tracer and performs the eviction on your behalf, allowing you to tune eviction policies separately from detection.

For clusters that need a turnkey path to automated evictions, we maintain a descheduler build that ships the `RemoveAnnotated` plugin. The plugin lives in the [eminaktas/descheduler `plugin/removeannotated` branch](https://github.com/eminaktas/descheduler/tree/plugin/removeannotated) and extends the upstream project with logic to evict pods carrying the annotation emitted by OOM Tracer (`oom-tracer.alpha.kubernetes.io/evict-me` by default).

- Container image: `ghcr.io/eminaktas/descheduler:v20250922-v0.34.0-removeannotated`
- Helm values example: [`docs/test-resources/descheduler-values.yaml`](docs/test-resources/descheduler-values.yaml)

The sample values configure the descheduler to enable the `RemoveAnnotated` plugin alongside the default evictor. Deploying the tracer with the matching `--victim-annotation` flag and running this descheduler build completes the feedback loop: annotated pods are recognized by the plugin and removed from the cluster automatically.

```bash
helm repo add descheduler https://kubernetes-sigs.github.io/descheduler/
helm upgrade --install descheduler \
  --namespace kube-system \
  descheduler/descheduler \
  -f docs/test-resources/descheduler-values.yaml \
  --version 0.33.0
```

The command above installs the upstream chart while overriding the container image so the `RemoveAnnotated` plugin is available to consume annotations emitted by OOM Tracer.

## Building the Agent

```bash
go generate ./...
go build -o bin/oom-tracer ./cmd
```

The build depends on architecture-specific `vmlinux` headers generated from the target kernel. If you upgrade the host kernel, regenerate the files under `pkg/ebpf/bpf/headers/` using `bpftool btf dump file /sys/kernel/btf/vmlinux format c` on the target machine. See `docs/DEVELOPMENT.md` for multi-architecture and Lima-based workflows as well as tips for automating header refreshes.

## Building a Container Image for Your Kernel

Because BPF bytecode is validated against the running kernel, you should compile the program against the BTF data from the same kernel version that will run the agent:

1. Collect the kernel's BTF data (`/sys/kernel/btf/vmlinux`) on the target distribution.
2. Regenerate the headers (`vmlinux_generated_<arch>.h`) and commit or bake them into your build context.
3. Build the image with your preferred toolchain (for example `docker buildx build --platform linux/amd64,linux/arm64 ...`).
4. Publish the image to a registry accessible by your cluster.

By repeating the above inside any Linux host (bare metal, VM, or CI runner) you can produce images that are globally distributable while staying kernel-compatible. The sample Lima workflow in `docs/DEVELOPMENT.md` shows one way to automate this for both amd64 and arm64.

## Deployment

A Helm chart is provided under `chart/oom-tracer/`. It deploys the tracer as a DaemonSet with a service account that has `get`, `list`, `watch`, and `patch` access to pods so that annotations can be applied. Ensure that the descheduler (or any other controller consuming the annotations) is deployed separately and configured with the same annotation key/value pair you pass to the tracer (`--victim-annotation`).

The tracer exposes Prometheus metrics at `/metrics` when `--metrics-bind-address` is set (default `:8080`).

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `events_total` | Counter | `stage`, `reason` | Lifecycle of observed OOM events (observed, identified, skipped, error). |
| `marked_victims_total` | Counter | `stage`, `reason` | Outcome of pod annotation attempts (marked, skipped). |

Label values are normalised to lower-case snake case to stay Prometheus-friendly. Inspect the `/metrics` output to discover the catalogue of reasons produced by the agent.

## References

- Chris Siebenmann, [*When does Linux invoke the OOM killer?*](https://utcc.utoronto.ca/~cks/space/blog/linux/OOMKillerWhen)
- Google Kubernetes Engine Documentation, [*System-level OOM kill*](https://cloud.google.com/kubernetes-engine/docs/troubleshooting/oom-events#system-oom-kill)
- Kubernetes SIG-Scheduling, [Descheduler project](https://github.com/kubernetes-sigs/descheduler)

## Runtime Flags

Run `oom-tracer --help` to inspect all flags. Notable options include:

- `--metrics-bind-address`: bind address for the Prometheus endpoint (empty string disables exposure).
- `--victim-annotation`: `key=value` pair to stamp on OOM victims so the `AnnotationEvictor` can evict them.
- Kubernetes connection flags under `--kubeconfig`, and client-go QPS/Burst settings inherited from component-base.

Pair the tracer with the Kubernetes manifests in `docs/test-resources/k8s/` and the node-level `docs/test-resources/oomhog.service` systemd unit to exercise both cgroup-scoped and global memory pressure flows before enabling automated evictions.
