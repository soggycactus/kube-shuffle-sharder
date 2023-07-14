package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type NodeGroup struct {
	NumNodes int
	Nodes    map[string]struct{}
}

type NodeGroupCollection map[string]NodeGroup

type PodMutatingWebhook struct {
	Config                      *rest.Config
	Mu                          *sync.Mutex
	Cache                       NodeGroupCollection
	NodeGroupAutoDiscoveryLabel string
	decoder                     *admission.Decoder
}

func (p *PodMutatingWebhook) Start(ctx context.Context) error {
	return p.StartInformer(ctx)
}

func (p *PodMutatingWebhook) StartInformer(ctx context.Context) error {
	logger := log.FromContext(ctx)

	clientset := kubernetes.NewForConfigOrDie(p.Config)
	factory := informers.NewSharedInformerFactory(clientset, 1*time.Minute)
	nodeInformer := factory.Core().V1().Nodes().Informer()

	stop := ctx.Done()

	defer runtime.HandleCrash()

	go factory.Start(stop)

	handle, err := nodeInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: p.filterFunc,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    p.AddFunc,
			UpdateFunc: p.UpdateFunc,
			DeleteFunc: p.DeleteFunc,
		},
	})
	if err != nil {
		return err
	}

	if !cache.WaitForCacheSync(stop, handle.HasSynced, nodeInformer.HasSynced) {
		err := errors.New("Timed out waiting for caches to sync")
		runtime.HandleError(err)
		return err
	}

	logger.Info("cache synced, informer started")

	<-stop

	return nil
}

func (p *PodMutatingWebhook) filterFunc(obj interface{}) bool {
	ctx := context.Background()
	logger := log.FromContext(ctx)

	object, err := meta.Accessor(obj)
	if err != nil {
		logger.Error(nil, ErrUnableToCastMeta.Error())
		return false
	}

	if _, ok := object.GetLabels()[p.NodeGroupAutoDiscoveryLabel]; !ok {
		logger.Info("skipping node, auto-discovery label not found", "missingLabel", p.NodeGroupAutoDiscoveryLabel, "nodeName", object.GetName())
		return false
	}

	return true
}

func (p *PodMutatingWebhook) AddFunc(obj interface{}) {
	ctx := context.Background()
	logger := log.FromContext(ctx)

	node, group, err := p.getGroupFromNode(obj)
	if err != nil {
		logger.Error(err, "failed to get group from node")
		return
	}

	p.addNodeIfNotExists(*group, node.Name)

	logger.Info("new node added", "name", node.Name, "group", group)
}

func (p *PodMutatingWebhook) UpdateFunc(oldObj, newObj interface{}) {
	ctx := context.Background()
	logger := log.FromContext(ctx)

	oldNode, oldGroup, err := p.getGroupFromNode(oldObj)
	if err != nil {
		logger.Error(err, "failed to get previous node state")
		return
	}

	newNode, newGroup, err := p.getGroupFromNode(newObj)
	if err != nil {
		logger.Error(err, "failed to get new node state")
		return
	}

	if *oldGroup == *newGroup {
		return
	}

	p.addNodeIfNotExists(*newGroup, newNode.Name)
	p.deleteNodeIfExists(*oldGroup, oldNode.Name)

	logger.Info("node moved to different group", "name", newNode.Name, "oldGroup", *oldGroup, "newGroup", *newGroup)
}

func (p *PodMutatingWebhook) DeleteFunc(obj interface{}) {
	ctx := context.Background()
	logger := log.FromContext(ctx)

	node, group, err := p.getGroupFromNode(obj)
	if err != nil {
		logger.Error(err, "failed to get group from node")
		return
	}

	p.deleteNodeIfExists(*group, node.Name)

	logger.Info("node removed from group", "name", node.Name, "group", group)
}

func (p *PodMutatingWebhook) addNodeIfNotExists(group, nodeName string) {
	p.Mu.Lock()
	defer p.Mu.Unlock()
	nodeGroup, ok := p.Cache[group]
	// if the node group doesn't exist, create it & initialize it with the node
	if !ok {
		p.Cache[group] = NodeGroup{
			NumNodes: 1,
			Nodes: map[string]struct{}{
				nodeName: {},
			},
		}
		return
	}

	// return if the node is already counted in the group
	_, ok = nodeGroup.Nodes[nodeName]
	if ok {
		return
	}

	nodeGroup.NumNodes += 1
	nodeGroup.Nodes[nodeName] = struct{}{}
	p.Cache[group] = nodeGroup
}

func (p *PodMutatingWebhook) deleteNodeIfExists(group, nodeName string) {
	p.Mu.Lock()
	defer p.Mu.Unlock()

	nodeGroup, ok := p.Cache[group]
	// if the node group doesn't exist, return
	if !ok {
		return
	}

	// if the node doesn't exist, return
	if _, ok := nodeGroup.Nodes[nodeName]; !ok {
		return
	}

	nodeGroup.NumNodes -= 1
	delete(nodeGroup.Nodes, nodeName)

	if nodeGroup.NumNodes == 0 {
		delete(p.Cache, group)
		return
	}

	p.Cache[group] = nodeGroup

}

func (p *PodMutatingWebhook) getGroupFromNode(obj interface{}) (*corev1.Node, *string, error) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil, nil, ErrUnableToCastNode
	}

	group, ok := node.Labels[p.NodeGroupAutoDiscoveryLabel]
	if !ok {
		return nil, nil, ErrMissingNodeAutoDiscoveryLabel
	}

	return node, &group, nil
}

func (p *PodMutatingWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}
	err := p.decoder.Decode(req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

func (p *PodMutatingWebhook) InjectDecoder(d *admission.Decoder) error {
	p.decoder = d
	return nil
}

func (p *PodMutatingWebhook) SetupWithManager(mgr ctrl.Manager) error {
	//mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhook.Admission{Handler: p})
	return mgr.Add(p)
}
