# OOM Tracer GitHub Pages

This is the branch where we publish Helm charts to GitHub Pages.

## About OOM Tracer

OOM Tracer observes global Linux OOM killer events on Kubernetes nodes, maps them back to pods, and annotates the offending workloads so that a descheduler instance can evict them. The intent is to reduce cascading failures that follow severe memory pressure by letting you plug the detection signal into an AnnotationEvictor downstream.

## Installation

To install OOM Tracer using Helm, add the Helm repository:

```bash
helm repo add oom-tracer https://eminaktas.github.io/oom-tracer/
helm repo update
helm install oom-tracer oom-tracer/oom-tracer --namespace oom-tracer-system --create-namespace
```

## Documentation

For complete documentation on OOM Tracer, including configuration options, examples, and advanced usage, refer to the [OOM Tracer README](https://github.com/eminaktas/oom-tracer?tab=readme-ov-file#oom-tracer).

## Contributing

We welcome contributions to OOM Tracer! If you’d like to contribute, please fork the repository and submit a pull request.

## License

This project is licensed under the [MIT License](LICENSE).
