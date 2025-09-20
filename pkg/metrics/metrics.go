package metrics

import (
	"errors"
	"net/http"
	"strings"

	"github.com/eminaktas/oom-tracer/pkg/version"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

const (
	MetricsSubsystem = "oom_tracer"
)

var buildInfo = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Subsystem:   MetricsSubsystem,
		Name:        "build_info",
		Help:        "Build info about oom-tracer, including Go version, oom-tracer version, Git SHA, Git branch",
		ConstLabels: map[string]string{"GoVersion": version.Get().GoVersion, "AppVersion": version.Get().Major + "." + version.Get().Minor, "OOMTracerVersion": version.Get().GitVersion, "GitSha1": version.Get().GitSha1},
	},
)

// Metrics bundles Prometheus counters used by the tracer.
type Metrics struct {
	events        *prometheus.CounterVec
	markedVictims *prometheus.CounterVec
	bindAddr      string
}

// NewMetrics registers counters and returns the bundle.
func NewMetrics(bindAddr string) (*Metrics, error) {
	if addr := strings.TrimSpace(bindAddr); addr != "" {
		m := &Metrics{
			events: prometheus.NewCounterVec(prometheus.CounterOpts{
				Subsystem: MetricsSubsystem,
				Name:      "events_total",
				Help:      "Total number of OOM kill events captured, bucketed by stage and reason.",
			}, []string{"stage", "reason"}),
			markedVictims: prometheus.NewCounterVec(prometheus.CounterOpts{
				Subsystem: MetricsSubsystem,
				Name:      "marked_victims_total",
				Help:      "Total number of marked pods to be evicted, bucketed by stage and reason.",
			}, []string{"stage", "reason"}),
			bindAddr: addr,
		}
		prometheus.MustRegister(m.events, m.markedVictims, buildInfo)
		return m, nil
	}
	return nil, nil
}

// IncEvent increments the event counter.
func (m *Metrics) IncEvent(stage, reason string) {
	if m == nil {
		return
	}
	increment(m.events, stage, reason)
}

// IncMarkedVictim increments the victim counter.
func (m *Metrics) IncMarkedVictim(stage, reason string) {
	if m == nil {
		return
	}
	increment(m.markedVictims, stage, reason)
}

// Inc increments the given counter.
func increment(counterVec *prometheus.CounterVec, stage, reason string) {
	if stage == "" {
		stage = "unknown"
	}
	if reason == "" {
		reason = "none"
	}
	counterVec.WithLabelValues(stage, reason).Inc()
}

func (m *Metrics) ServeMux() {
	if m == nil {
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(m.bindAddr, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
			klog.ErrorS(err, "Metrics server exited", "address", m.bindAddr)
		}
	}()
	klog.InfoS("Metrics endpoint listening", "address", m.bindAddr)
}
