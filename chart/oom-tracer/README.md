# OOM Tracer for Kubernetes

OOM Tracer observes global Linux OOM killer events on Kubernetes nodes, maps them back to pods, and annotates the offending workloads so that a descheduler instance can evict them. The intent is to reduce cascading failures that follow severe memory pressure by letting you plug the detection signal into an AnnotationEvictor downstream.

## TL;DR:

```shell
helm repo add oom-tracer https://eminaktas.github.io/oom-tracer/
helm repo update
helm install my-release oom-tracer/oom-tracer --namespace kube-system
```

## Documentation

For complete documentation on OOM Tracer, including configuration options, examples, and advanced usage, refer to the [OOM Tracer README](https://github.com/eminaktas/oom-tracer?tab=readme-ov-file#oom-tracer).

## Installing the Chart

To install the chart with the release name `my-release`:

```shell
helm install my-release oom-tracer/oom-tracer --namespace kube-system
```

The command deploys _oom-tracer_ on the Kubernetes cluster in the default configuration.

> **Tip**: List all releases using `helm list`

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```shell
helm delete my-release
```

The command removes all the Kubernetes components associated with the chart and deletes the release.
