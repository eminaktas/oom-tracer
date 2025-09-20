package oomtracer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/eminaktas/oom-tracer/pkg/ebpf"
	"github.com/eminaktas/oom-tracer/pkg/k8s"
	"github.com/eminaktas/oom-tracer/pkg/metrics"
	"golang.org/x/sys/unix"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

const (
	eventStageObserved   = "observed"
	eventStageIdentified = "identified"
	eventStageSkipped    = "skipped"
	eventStageError      = "error"

	victimStageMarked  = "marked"
	victimStageSkipped = "skipped"
)

// ID keeps the identity of the Pod that is the victim of an OOM kill.
type ID struct {
	// Namespace of the Pod.
	Namespace string
	// Name of the Pod.
	Name string
	// Unique identifier (UID) of the Pod.
	UID types.UID
	// Annotations assigned to the Pod.
	Annotations map[string]string
}

// OOMTracer provides facilities to trace Out-Of-Memory (OOM) kill events
// in the cluster.
type OOMTracer struct {
	k8s                 *k8s.K8s
	nodeName            string
	metrics             *metrics.Metrics
	victimAnnotationKey string
	victimAnnotationVal string
	ignoreGlobalOOM     bool
}

func NewOOMTracer(k8s *k8s.K8s, nodeName string, metrics *metrics.Metrics, victimAnnotation string, ignoreGlobalOOM bool) (*OOMTracer, error) {
	key, val, err := parseAnnotation(victimAnnotation)
	if err != nil {
		return nil, err
	}

	return &OOMTracer{
		k8s:                 k8s,
		nodeName:            nodeName,
		metrics:             metrics,
		victimAnnotationKey: key,
		victimAnnotationVal: val,
		ignoreGlobalOOM:     ignoreGlobalOOM,
	}, nil
}

func (o *OOMTracer) TraceKernelOOMKills(ctx context.Context) error {
	rd, closeFn, err := ebpf.LoadBPF("oom_kill_process")
	if err != nil {
		return err
	}
	defer closeFn()

	// Close reader on cancellation
	go func() {
		<-ctx.Done()
		_ = rd.Close() // // triggers ringbuf.ErrClosed in loop
	}()

	klog.InfoS("eBPF program loaded; listening for OOM kill events")

	for {
		record, err := rd.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return ctx.Err()
			}
			klog.ErrorS(err, "Error reading from ring buffer")
			continue
		}

		var event ebpf.BpfEvent
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			klog.ErrorS(err, "Failed to parse ring buffer event")
			continue
		}

		o.metrics.IncEvent(eventStageObserved, "ebpf_capture")

		vcommand := Int8SliceToString(event.VictimComm[:])
		tcommand := Int8SliceToString(event.TriggerComm[:])
		candidates := Int8SliceToStringSlice(event.Names[:])

		klog.InfoS("OOM captured",
			"trigger_pid", event.TriggerPid,
			"trigger_command", tcommand,
			"victim_pid", event.VictimPid,
			"victim_command", vcommand,
			"totalpages", event.Totalpages,
			"globaloom", event.GlobalOom,
		)

		if !o.ignoreGlobalOOM && !event.GlobalOom {
			klog.InfoS("Captured OOM is not global while global-only mode is active. Won't take action", "globaloom", event.GlobalOom)
			o.metrics.IncEvent(eventStageSkipped, "non_or_ignored_global_oom")
			continue
		}

		if candidates != nil {
			pods, err := o.k8s.ListPodsOnANode(o.nodeName, nil)
			if err != nil {
				return err
			}

			var id *ID
			for _, s := range candidates {
				if s == "" {
					continue
				}
				id = LookupPod(s, pods)
				if id != nil {
					break
				}
			}

			if id == nil {
				klog.InfoS("No pod match found for victim", "candidates", candidates)
				o.metrics.IncEvent(eventStageSkipped, "no_pod_match")
				continue
			}
			o.metrics.IncEvent(eventStageIdentified, "pod_identified")

			if o.victimAnnotationKey != "" {
				if _, ok := id.Annotations[o.victimAnnotationKey]; !ok {
					annotations := map[string]string{o.victimAnnotationKey: o.victimAnnotationVal}
					if err := o.k8s.AnnotatePod(ctx, id.Namespace, id.Name, annotations); err != nil {
						klog.ErrorS(err, "Failed to handle OOM victim",
							"namespace", id.Namespace,
							"name", id.Name,
						)
						o.metrics.IncEvent(eventStageError, "annotation_failed")
						continue
					}

					klog.InfoS("Eviction marked for OOM victim",
						"namespace", id.Namespace,
						"name", id.Name,
						"uid", id.UID,
						"trigger_pid", event.TriggerPid,
						"victim_pid", event.VictimPid,
					)
					o.metrics.IncMarkedVictim(victimStageMarked, "annotation_applied")
					continue
				}
				klog.InfoS("Victim is already marked to be evicted",
					"namespace", id.Namespace,
					"name", id.Name,
					"uid", id.UID,
					"trigger_pid", event.TriggerPid,
					"victim_pid", event.VictimPid,
				)
				o.metrics.IncMarkedVictim(victimStageSkipped, "annotation_exists")
			}
		}
	}
}

func LookupPod(pcid string, pods []*corev1.Pod) *ID {
	podUIDMatch := podPattern.FindStringSubmatch(pcid)
	containerIDMatch := cidPattern.FindStringSubmatch(pcid)
	if podUIDMatch == nil && containerIDMatch == nil {
		return nil
	}

	for _, pod := range pods {
		if podUIDMatch != nil {
			podUID := strings.ReplaceAll(podUIDMatch[1], "_", "-")
			if string(pod.UID) == podUID {
				return &ID{
					Namespace:   pod.GetNamespace(),
					Name:        pod.GetName(),
					UID:         pod.GetUID(),
					Annotations: pod.Annotations,
				}
			}
		} else if containerIDMatch != nil {
			target := containerIDMatch[0]
			for _, status := range pod.Status.ContainerStatuses {
				if live := cidPattern.FindString(status.ContainerID); live != "" && live == target {
					return &ID{Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID, Annotations: pod.Annotations}
				}
			}
			for _, status := range pod.Status.InitContainerStatuses {
				if live := cidPattern.FindString(status.ContainerID); live != "" && live == target {
					return &ID{Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID, Annotations: pod.Annotations}
				}
			}
			for _, status := range pod.Status.EphemeralContainerStatuses {
				if live := cidPattern.FindString(status.ContainerID); live != "" && live == target {
					return &ID{Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID, Annotations: pod.Annotations}
				}
			}
		}
	}
	return nil
}

var podPattern = regexp.MustCompile(`pod([a-f0-9_-]+)`)
var cidPattern = regexp.MustCompile(`[a-f0-9]{64}`)

// Int8SliceToString copies an arbitrary []int8 (or an array you slice) to string.
func Int8SliceToString(src []int8) string {
	b := make([]byte, len(src))
	for i, v := range src {
		b[i] = byte(v)
	}
	return unix.ByteSliceToString(b)
}

// Int8SliceToStringSlice copies an arbitrary [][]int8 (or an array you slice) to string slice.
func Int8SliceToStringSlice(src [][128]int8) []string {
	a := make([]string, len(src))
	for y, x := range src {
		b := make([]byte, len(x))
		for i, v := range x {
			b[i] = byte(v)
		}
		a[y] = unix.ByteSliceToString(b)
	}

	return a
}

func parseAnnotation(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", nil
	}
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid annotation %q, expected key=value", raw)
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	if key == "" {
		return "", "", fmt.Errorf("annotation key cannot be empty")
	}
	return key, val, nil
}
