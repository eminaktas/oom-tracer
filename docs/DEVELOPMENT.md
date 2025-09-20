# Development

This guide describes how to build, test, and package OOM Tracer across different architectures while keeping the eBPF artifacts compatible with the Linux kernels you operate.

## Prerequisites

- Go 1.25 or newer
- A recent Clang/LLVM toolchain (required for compiling eBPF bytecode)
- `bpftool` for extracting BTF information from the target kernel
- Docker or another OCI image builder (optional, only if you publish images)

On Debian/Ubuntu style systems you can install the toolchain with:

```bash
sudo apt-get update
sudo apt-get install -y clang llvm make build-essential linux-tools-common linux-tools-generic linux-tools-$(uname -r)
```

## Building the Binary

Regenerate the architecture-specific headers before compiling if you upgraded your kernel:

```bash
bpftool btf dump file /sys/kernel/btf/vmlinux format c > pkg/ebpf/bpf/headers/vmlinux_generated_$(uname -m).h
sed -i s/__VMLINUX_H__/__VMLINUX_$(uname -m | tr '[:lower:]' '[:upper:]')_H__/ pkg/ebpf/bpf/headers/vmlinux_generated_$(uname -m).h
```

Then build the agent:

```bash
go generate ./...
go build -o bin/oom-tracer ./cmd
```

The resulting binary listens for OOM events and can run directly on a node once you configure the runtime flags described in the `README`.

## Kernel-Compatible Container Images

Because eBPF verification is kernel-specific, ship images that were built against the same (or a sufficiently similar) kernel BTF data. A portable workflow is:

1. Start a Linux VM or bare-metal host that matches the target kernel.
2. Regenerate the `vmlinux_generated_<arch>.h` header on that machine.
3. Build the image locally and push it to your registry.

For multi-architecture builds you can stitch multiple builders together with Docker Buildx:

```bash
# Create rootful Lima VMs (optional helper for macOS hosts)
limactl start --name=docker-amd64 --arch x86_64 template://docker-rootful
limactl start --name=docker-arm64 --arch aarch64 template://docker-rootful

# Aggregate them into a single builder
docker buildx create --name multiarch-builder --use lima-docker-amd64
docker buildx create --append --name multiarch-builder lima-docker-arm64

docker buildx inspect --bootstrap
```

Build and optionally push the image once the builder is ready:

```bash
docker buildx build \
  --builder multiarch-builder \
  --platform linux/amd64,linux/arm64 \
  --tag your-org/oom-tracer:<VERSION> \
  --push .
```

Swap the Lima steps with any other infrastructure that gives you access to the kernels you care about (for example, dedicated build nodes in CI or cloud-hosted runners).

## Updating Generated Headers

Store regenerated headers under `pkg/ebpf/bpf/headers/` using the naming convention already in the repository:

- `vmlinux_generated_arm64.h`
- `vmlinux_generated_x86.h`

Regenerate after kernel upgrades or when enabling new eBPF features.

## Observability and Local Testing

- Enable metrics by passing `--metrics-bind-address=:8080` and scrape `/metrics` with Prometheus during tests. The label values reported by the tracer are already Prometheus-friendly (`snake_case`).
- Follow [TEST.md](TEST.md) for Kubernetes manifests in `docs/test-resources/k8s/` and node-level stress tools that help rehearse the full detection/eviction loop. Start in dry-run mode (empty `--victim-annotation`) to confirm visibility before letting the descheduler evict pods automatically.

Keeping these practices in your workflow helps ensure that the tracer remains portable across regions and kernel variants.
