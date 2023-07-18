package controller

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"sync"
	"time"

	v1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	"github.com/soggycactus/kube-shuffle-sharder/shuffleshard"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type NodeGroup struct {
	NumNodes int
	Nodes    map[string]struct{}
}

type NodeGroupCollection map[string]NodeGroup

// +kubebuilder:webhook:admissionReviewVersions=v1,sideEffects=None,path=/mutate-v1-pod,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create;update,versions=v1,name=webhook.kube-shuffle-sharder.io

type PodMutatingWebhook struct {
	Config                      *rest.Config
	Client                      client.Client
	Mu                          *sync.Mutex
	Cache                       NodeGroupCollection
	NodeGroupAutoDiscoveryLabel string
	TenantLabel                 string
	NumNodeGroups               int
	decoder                     *admission.Decoder
}

// Start fulfills the manager.Runnable interface,
// required when calling (manager.Manager).Add in
// (controller.PodMutatingWebhook).SetupWithManager
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

	// inspect label & search for shuffle shard if it exists
	tenant, ok := pod.Labels[p.TenantLabel]
	if !ok {
		return admission.Errored(http.StatusBadRequest, ErrMissingTenantLabel)
	}

	shardList := v1.ShuffleShardList{}
	err = p.Client.List(ctx, &shardList, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.tenant": tenant,
		}),
	})

	// If the error is any error other than not found, return an error
	if client.IgnoreNotFound(err) != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var nodeGroups []string
	// If the shard list is not empty, use that for the node groups
	// otherwise shuffle shard a new group
	if len(shardList.Items) != 0 {
		nodeGroups = shardList.Items[0].Spec.NodeGroups
	} else {
		groups, err := p.ShuffleShard(ctx, p.NumNodeGroups)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		nodeGroups = groups
	}

	// Patch the pod's current node affinity
	currentNodeAffinity := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	currentNodeAffinity.NodeSelectorTerms = append(currentNodeAffinity.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      p.NodeGroupAutoDiscoveryLabel,
				Operator: corev1.NodeSelectorOpIn,
				Values:   nodeGroups,
			},
		},
	})

	pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = currentNodeAffinity

	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

func (p *PodMutatingWebhook) ShuffleShard(ctx context.Context, numNodeGroups int) ([]string, error) {
	p.Mu.Lock()

	nodeGroups := []string{}
	for key := range p.Cache {
		nodeGroups = append(nodeGroups, key)
	}

	p.Mu.Unlock()

	sharder := shuffleshard.Sharder[string]{
		Endpoints:         nodeGroups,
		ReplicationFactor: numNodeGroups,
		ShardKeyFunc:      HashShard,
		ShardStore:        p,
		Rand:              rand.New(rand.NewSource(time.Now().Unix())),
	}

	return sharder.ShuffleShard(ctx)
}

func (p *PodMutatingWebhook) ShardExists(ctx context.Context, shardHash string) (bool, error) {
	var shuffleShardList v1.ShuffleShardList
	if err := p.Client.List(ctx, &shuffleShardList, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"status.shardHash": shardHash,
		}),
	}); client.IgnoreNotFound(err) != nil {
		// return true in case the caller doesn't check the err
		// to force another iteration of backtracking
		return true, err
	}

	return true, nil
}

// InjectDecoder provides a method for an admission.Decoder to be set;
// controller-runtime will automatically inject a decoder using this method
func (p *PodMutatingWebhook) InjectDecoder(d *admission.Decoder) error {
	p.decoder = d
	return nil
}

// SetupWithManager registers the handler with manager's webhook server
// and adds the informer to the list of processes for manager to start
func (p *PodMutatingWebhook) SetupWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhook.Admission{Handler: p})
	return mgr.Add(p)
}
