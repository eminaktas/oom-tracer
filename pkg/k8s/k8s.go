package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	componentbaseconfig "k8s.io/component-base/config"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

type K8s struct {
	ClientConnection componentbaseconfig.ClientConnectionConfiguration

	client                    kubernetes.Interface
	nodeLister                listersv1.NodeLister
	getPodsAssignedToNodeFunc podutil.GetPodsAssignedToNodeFunc
}

// CreateClients connects to Kubernetes, wires informers, and warms their caches.
func (k8s *K8s) CreateClients(ctx context.Context) error {
	// Build either in-cluster or from kubeconfig if set
	config, err := clientcmd.BuildConfigFromFlags("", k8s.ClientConnection.Kubeconfig)
	if err != nil {
		return err
	}

	config.Burst = int(k8s.ClientConnection.Burst)
	config.QPS = k8s.ClientConnection.QPS

	// create the clientset
	k8s.client, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	sharedInformerFactory := informers.NewSharedInformerFactory(k8s.client, 0)

	k8s.nodeLister = sharedInformerFactory.Core().V1().Nodes().Lister()
	k8s.getPodsAssignedToNodeFunc, err = podutil.BuildGetPodsAssignedToNodeFunc(sharedInformerFactory.Core().V1().Pods().Informer())
	if err != nil {
		return err
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	return nil
}

// ListPodsOnANode lists pods assigned to the node, optionally filtered via podutil.FilterFunc.
func (k8s *K8s) ListPodsOnANode(nodeName string, filter podutil.FilterFunc) ([]*corev1.Pod, error) {
	return podutil.ListPodsOnANode(nodeName, k8s.getPodsAssignedToNodeFunc, filter)
}

// AnnotatePod applies the provided annotations using a merge patch.
func (k8s *K8s) AnnotatePod(ctx context.Context, namespace, name string, annotations map[string]string) error {
	if len(annotations) == 0 {
		return nil
	}
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": annotations,
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}
	_, err = k8s.client.CoreV1().Pods(namespace).Patch(ctx, name, types.MergePatchType, data, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patch pod %s/%s: %w", namespace, name, err)
	}
	return nil
}
