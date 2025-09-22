# Testing Global OOM Scenarios

Use these recipes to create realistic memory pressure and validate that OOM Tracer captures events end-to-end before allowing automated evictions.

## Descheduler With `RemoveAnnotated`

Pair the tracer with the descheduler build published at `ghcr.io/eminaktas/descheduler:v20250922-v0.34.0-removeannotated`. It includes the custom [`RemoveAnnotated` plugin](https://github.com/eminaktas/descheduler/tree/plugin/removeannotated), which evicts pods as soon as they receive the `oom-tracer.alpha.kubernetes.io/evict-me` annotation written by the tracer.

An example Helm values file lives at `docs/test-resources/descheduler-values.yaml`. Apply it with your preferred Helm workflow to run the descheduler alongside the tracer while rehearsing the flows below.

```bash
helm repo add descheduler https://kubernetes-sigs.github.io/descheduler/
helm upgrade --install descheduler \
  --namespace kube-system \
  descheduler/descheduler \
  -f docs/test-resources/descheduler-values.yaml \
  --version 0.33.0
```

## Kubernetes Workloads

The manifests under `docs/test-resources/k8s/` generate controlled memory pressure inside the cluster.

### 1. Unlimited Stress Pod

`docs/test-resources/k8s/unlimited-static-workload.yaml` runs a pod that continuously allocates memory until it is killed. Deploy it to any namespace:

```bash
kubectl apply -f docs/test-resources/k8s/unlimited-static-workload.yaml
```

Expect the pod to eventually reach an `OOMKilled` state.

### 2. Memory-Limited Pod

`docs/test-resources/k8s/limited-workload.yaml` applies an aggressive memory limit. Adjust the `resources.limits.memory` value to rehearse different eviction thresholds:

```bash
kubectl apply -f docs/test-resources/k8s/limited-workload.yaml
```

### 3. Static Pod Variant

`docs/test-resources/k8s/limited-static-workload.yaml` can be placed on `/etc/kubernetes/manifests` to pressure specific nodes when running kubelet-managed static pods.

### Cleanup

```bash
kubectl delete -f docs/test-resources/k8s/
```

## Node-Level Global OOM

For validation that the tracer records *global* (host-wide) OOM kills, deploy the `oomhog.service` unit on a test node. It relies on `stress-ng` to exhaust memory outside of Kubernetes cgroups.

### Install the Service

1. Copy the unit and reload systemd:
   ```bash
   sudo cp docs/test-resources/service/oomhog.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```
2. Enable and start the stressor:
   ```bash
   sudo systemctl enable --now oomhog.service
   ```

The service launches `stress-ng --vm 2 --vm-bytes 80% --vm-keep` and restarts automatically. Tweak the arguments (or uncomment `MemoryMax`) to match node capacity.

### Tear Down

- When finished, disable and remove the unit:
  ```bash
  sudo systemctl disable --now oomhog.service
  sudo rm /etc/systemd/system/oomhog.service
  sudo systemctl daemon-reload
  ```

Combine the node-level stressor with the tracer in dry-run mode first to confirm that global OOMs are detected and the Prometheus metrics increment as expected.
