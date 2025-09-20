//go:build linux

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eminaktas/oom-tracer/pkg/k8s"
	"github.com/eminaktas/oom-tracer/pkg/metrics"
	"github.com/eminaktas/oom-tracer/pkg/oomtracer"
	"github.com/eminaktas/oom-tracer/pkg/version"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/util/homedir"
	"k8s.io/component-base/config"
	"k8s.io/klog/v2"
)

func main() {
	var kubeConfig *string
	var nodeName *string
	var kubeClientQPS *float64
	var kubeClientBurst *int
	var metricsBindAddress *string
	var victimAnnotation *string
	var ignoreGlobalOOM *bool
	var printVersion *bool

	home := ""
	if home = homedir.HomeDir(); home != "" {
		home = filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(home); err != nil {
			home = ""
		}
	}
	kubeConfig = flag.String("kube_config", home, "absolute path to the kubeconfig file; empty uses in-cluster config")
	kubeClientQPS = flag.Float64("kube_client_qps", 50, "total number of queries per second")
	kubeClientBurst = flag.Int("kube_client_burst", 100, "extra query to accumulate when a client is exceeding its rate")
	nodeName = flag.String("node_name", os.Getenv("K8S_NODENAME"), "name of the Kubernetes node; empty uses K8S_NODENAME env var or kills the program")
	printVersion = flag.Bool("version", false, "prints the version and exits the program")
	metricsBindAddress = flag.String("metrics_bind_address", ":8080", "address to bind the Prometheus /metrics endpoint; empty disables the endpoint.")
	victimAnnotation = flag.String("victim_annotation", "", "annotation to add to OOM victims in key= or key=anyvalue form; empty runs in dry-run mode.")
	ignoreGlobalOOM = flag.Bool("ignore_global_oom", false, "act on OOM kills even when they are not classified as system-wide")
	klog.InitFlags(nil)
	flag.Parse()

	defer klog.Flush()

	if *printVersion {
		fmt.Printf("OOM Tracer version %+v\n", version.Get())
		os.Exit(0)
	}

	if *nodeName == "" {
		klog.Fatal("Either the --node_name flag or the K8S_NODENAME environment variable must be provided.")
	}

	// Root context canceled by SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	k8s := &k8s.K8s{
		ClientConnection: config.ClientConnectionConfiguration{
			Kubeconfig: *kubeConfig,
			QPS:        float32(*kubeClientQPS),
			Burst:      int32(*kubeClientBurst),
		},
	}
	// Setup K8s client
	err := k8s.CreateClients(ctx)
	if err != nil {
		klog.Fatalf("Failed to initialize Kubernetes client: %v", err)
	}

	metrics, err := metrics.NewMetrics(*metricsBindAddress)
	if err != nil {
		klog.ErrorS(err, "Failed to initialise metrics")
	}

	metrics.ServeMux()

	oomTracer, err := oomtracer.NewOOMTracer(
		k8s,
		*nodeName,
		metrics,
		*victimAnnotation,
		*ignoreGlobalOOM,
	)
	if err != nil {
		klog.Fatalf("Failed to initialise tracer: %v", err)
	}

	// Run until ctx is cancelled
	if err := oomTracer.TraceKernelOOMKills(ctx); err != nil && !errors.Is(err, context.Canceled) {
		klog.Fatalf("Tracer exited with error: %v", err)
	}
	klog.InfoS("Shutting down")
}
