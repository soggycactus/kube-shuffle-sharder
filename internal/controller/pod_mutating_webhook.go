package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	v1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	"github.com/soggycactus/kube-shuffle-sharder/shuffleshard"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	shuffleShardDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:      "shuffle_shard_duration_seconds",
			Namespace: PrometheusNamespace,
			Buckets: []float64{
				.025,
				.050,
				.100,
				.150,
				.200,
				.300,
				.400,
				.500,
				.750,
				1,
				2,
				5,
			},
		},
	)
	nodeGroupSizeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "node_group_size",
			Namespace: PrometheusNamespace,
		},
		[]string{"node_group"},
	)
	totalNodesGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:      "total_num_nodes",
			Namespace: PrometheusNamespace,
		},
	)
	totalNodeGroupsGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Name:      "total_num_node_groups",
		},
	)
	shuffleShardsUsedGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:      "num_shuffle_shards_used",
			Namespace: PrometheusNamespace,
		},
	)
	totalPossibleShards = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:      "num_shuffle_shards_possible",
			Namespace: PrometheusNamespace,
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		shuffleShardDuration,
		nodeGroupSizeGauge,
		totalNodesGauge,
		totalNodeGroupsGauge,
		shuffleShardsUsedGauge,
		totalPossibleShards,
	)
}

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
	Decoder                     *admission.Decoder
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

	go p.exportMetrics(stop)

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
	nodeGroupSizeGauge.With(prometheus.Labels{"node_group": *group}).Inc()
	totalNodesGauge.Inc()
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

	nodeGroupSizeGauge.With(prometheus.Labels{"node_group": *newGroup}).Inc()
	nodeGroupSizeGauge.With(prometheus.Labels{"node_group": *oldGroup}).Dec()
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

	nodeGroupSizeGauge.With(prometheus.Labels{"node_group": *group}).Dec()
	totalNodesGauge.Dec()
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
		totalNodeGroupsGauge.Inc()
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
		totalNodeGroupsGauge.Dec()
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
	logger := log.FromContext(ctx)

	pod := &corev1.Pod{}
	err := p.Decoder.Decode(req, pod)
	if err != nil {
		logger.Error(err, "failed to decode request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	// inspect label & search for shuffle shard if it exists
	tenant, ok := pod.Labels[p.TenantLabel]
	if !ok {
		logger.Error(ErrMissingTenantLabel, "failed to handle pod")
		return admission.Errored(http.StatusBadRequest, ErrMissingTenantLabel)
	}

	// Find the existing shard for the tenant, if it exists
	shuffleShard := v1.ShuffleShard{}
	// If the error is any error other than not found, return an error
	if err = p.Client.Get(ctx, types.NamespacedName{Name: tenant}, &shuffleShard); client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to get ShuffleShard")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var nodeGroups []string
	if shuffleShard.Spec.NodeGroups != nil {
		nodeGroups = shuffleShard.Spec.NodeGroups
	} else {
		groups, err := p.ShuffleShard(ctx, tenant, p.NumNodeGroups)
		if err != nil {
			logger.Error(err, "failed to create ShuffleShard")
			return admission.Errored(http.StatusInternalServerError, err)
		}

		nodeGroups = groups
	}

	// Create the nodeSelectorTerm to patch
	nodeSelectorTerm := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      p.NodeGroupAutoDiscoveryLabel,
				Operator: corev1.NodeSelectorOpIn,
				Values:   nodeGroups,
			},
		},
	}

	// Unfortunately, we need to check for nil pointers on the pod's Affinity
	// Appending our node groups to a nil NodeAffinity will cause a panic
	switch {
	case pod.Spec.Affinity == nil:
		pod.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						nodeSelectorTerm,
					},
				},
			},
		}
	case pod.Spec.Affinity.NodeAffinity == nil:
		pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					nodeSelectorTerm,
				},
			},
		}
	case pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil:
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				nodeSelectorTerm,
			},
		}

	case pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms == nil:
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []corev1.NodeSelectorTerm{
			nodeSelectorTerm,
		}

	default:
		currentNodeAffinity := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		currentNodeAffinity.NodeSelectorTerms = append(currentNodeAffinity.NodeSelectorTerms, nodeSelectorTerm)
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = currentNodeAffinity
	}

	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		logger.Error(err, "failed to marshal pod")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

func (p *PodMutatingWebhook) ShuffleShard(ctx context.Context, tenant string, numNodeGroups int) ([]string, error) {
	p.Mu.Lock()
	defer p.Mu.Unlock()

	timer := prometheus.NewTimer(shuffleShardDuration)
	defer timer.ObserveDuration()

	nodeGroups := []string{}
	for key := range p.Cache {
		nodeGroups = append(nodeGroups, key)
	}

	sharder := shuffleshard.Sharder[string]{
		Endpoints:         nodeGroups,
		ReplicationFactor: numNodeGroups,
		ShardKeyFunc:      HashShard,
		ShardStore:        p,
		Rand:              rand.New(rand.NewSource(time.Now().Unix())),
	}

	groups, err := sharder.ShuffleShard(ctx)
	if err != nil {
		return nil, err
	}

	shuffleShard := v1.ShuffleShard{
		ObjectMeta: metav1.ObjectMeta{
			Name: tenant,
		},
		Spec: v1.ShuffleShardSpec{
			Tenant:     tenant,
			NodeGroups: groups,
		},
	}
	if err := p.Client.Create(ctx, &shuffleShard); err != nil {
		return nil, err
	}

	return groups, nil
}

func (p *PodMutatingWebhook) ShardExists(ctx context.Context, shardHash string) (bool, error) {
	var shuffleShardList v1.ShuffleShardList
	if err := p.Client.List(ctx, &shuffleShardList, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"status.shardHash": shardHash,
		}),
	}); err != nil {
		// return true in case the caller doesn't check the err
		// to force another iteration of backtracking
		return true, err
	}

	// if no shards exist, return false
	if len(shuffleShardList.Items) == 0 {
		return false, nil
	}

	return true, nil
}

// SetupWithManager registers the handler with manager's webhook server
// and adds the informer to the list of processes for manager to start
func (p *PodMutatingWebhook) SetupWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhook.Admission{Handler: p, RecoverPanic: true})
	return mgr.Add(p)
}

func (p *PodMutatingWebhook) exportMetrics(done <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	logger := log.FromContext(context.Background())

	exportMetrics := func() {
		combinations, err := Choose(len(p.Cache), p.NumNodeGroups)
		if err != nil {
			logger.Error(err, "failed to update total possible shards")
		}

		if combinations != nil {
			totalPossibleShards.Set(float64(*combinations))
		}

		var shuffleShardList v1.ShuffleShardList
		if err := p.Client.List(context.Background(), &shuffleShardList, &client.ListOptions{}); err != nil {
			logger.Error(err, "failed to update total shards used")
			return
		}

		shuffleShardsUsedGauge.Set(float64(len(shuffleShardList.Items)))
	}

	// initialize metrics
	exportMetrics()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			exportMetrics()
		}
	}
}

// Choose calculates n choose k - the number of possible combinations
func Choose(n, k int) (*int, error) {
	pointerInt := func(i int) *int {
		return &i
	}

	if k > n {
		return nil, fmt.Errorf("cannot have k (%d) greater than n (%d)", k, n)
	}
	if k < 0 {
		return nil, fmt.Errorf("cannot have k (%d) less than 0", k)
	}
	if n <= 1 || k == 0 || n == k {
		return pointerInt(1), nil
	}
	if newK := n - k; newK < k {
		k = newK
	}
	if k == 1 {
		return pointerInt(n), nil
	}
	// Our return value, and this allows us to skip the first iteration.
	ret := n - k + 1
	for i, j := ret+1, 2; j <= k; i, j = i+1, j+1 {
		ret = ret * i / j
	}
	return pointerInt(ret), nil
}
