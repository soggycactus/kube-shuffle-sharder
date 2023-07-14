package controller_test

import (
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/soggycactus/kube-shuffle-sharder/internal/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	autoDiscoveryLabel = "kube-shuffle-sharder.io/node-group"
)

func TestEventHandlerFuncs(t *testing.T) {
	p := controller.PodMutatingWebhook{
		Mu:                          new(sync.Mutex),
		Cache:                       make(controller.NodeGroupCollection),
		NodeGroupAutoDiscoveryLabel: autoDiscoveryLabel,
	}

	nodes := map[string]*corev1.Node{
		"node-1": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					autoDiscoveryLabel: "group-a",
				},
			},
		},
		"node-2": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					autoDiscoveryLabel: "group-b",
				},
			},
		},
		"node-3": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
				Labels: map[string]string{
					autoDiscoveryLabel: "group-b",
				},
			},
		},
		"node-4": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-4",
				Labels: map[string]string{
					autoDiscoveryLabel: "group-c",
				},
			},
		},
		"node-5": {
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-5",
				Labels: map[string]string{
					autoDiscoveryLabel: "group-d",
				},
			},
		},
	}

	for _, node := range nodes {
		p.AddFunc(node)
	}

	Expect(p.Cache["group-a"].NumNodes).To(Equal(1))
	Expect(p.Cache["group-b"].NumNodes).To(Equal(2))
	Expect(p.Cache["group-c"].NumNodes).To(Equal(1))
	Expect(p.Cache["group-d"].NumNodes).To(Equal(1))
}
