package controller_test

import (
	"context"
	"sync"
	"testing"

	v1 "github.com/soggycactus/kube-shuffle-sharder/api/v1"
	"github.com/soggycactus/kube-shuffle-sharder/internal/controller"
	"github.com/soggycactus/kube-shuffle-sharder/pkg/graph"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	autoDiscoveryLabel = "kube-shuffle-sharder.io/node-group"
)

func TestNodeEventHandlerFuncs(t *testing.T) {
	p := controller.PodMutatingWebhook{
		Mu:                          new(sync.Mutex),
		NodeCache:                   make(controller.NodeGroupCollection),
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
		p.NodeAddFunc(node)
	}

	assert.Equal(t, 1, p.NodeCache["group-a"].NumNodes, "node count should match")
	assert.Equal(t, 2, p.NodeCache["group-b"].NumNodes, "node count should match")
	assert.Equal(t, 1, p.NodeCache["group-c"].NumNodes, "node count should match")
	assert.Equal(t, 1, p.NodeCache["group-d"].NumNodes, "node count should match")

	oldNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-4",
			Labels: map[string]string{
				autoDiscoveryLabel: "group-c",
			},
		},
	}
	newNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-4",
			Labels: map[string]string{
				autoDiscoveryLabel: "group-d",
			},
		},
	}
	p.NodeUpdateFunc(oldNode, newNode)
	_, ok := p.NodeCache["group-c"]
	assert.False(t, ok, "group-c should be missing")
	assert.Equal(t, 2, p.NodeCache["group-d"].NumNodes, "node should have moved to group-d")

	p.NodeDeleteFunc(newNode)
	assert.Equal(t, 1, p.NodeCache["group-d"].NumNodes, "node should have been removed from group-d")

}

func TestShardHandlerFuncs(t *testing.T) {
	p := controller.PodMutatingWebhook{
		Mu:            new(sync.Mutex),
		EndpointGraph: graph.NewGraph[string](),
	}

	shuffleShards := []*v1.ShuffleShard{
		{
			Spec: v1.ShuffleShardSpec{
				Tenant: "tenant-1",
				NodeGroups: []string{
					"group-b",
					"group-c",
					"group-f",
				},
			},
			Status: v1.ShuffleShardStatus{
				ShardHash: "bcf",
			},
		},
		{
			Spec: v1.ShuffleShardSpec{
				Tenant: "tenant-2",
				NodeGroups: []string{
					"group-c",
					"group-d",
					"group-a",
				},
			},
			Status: v1.ShuffleShardStatus{
				ShardHash: "cda",
			},
		},
		{
			Spec: v1.ShuffleShardSpec{
				Tenant: "tenant-3",
				NodeGroups: []string{
					"group-a",
					"group-d",
					"group-e",
				},
			},
			Status: v1.ShuffleShardStatus{
				ShardHash: "ade",
			},
		},
	}

	for _, shard := range shuffleShards {
		p.ShardAddFunc(shard)
	}

	// group-a has shards ade & cda
	assert.Equal(t, 3, p.EndpointGraph.NumEdges("group-a"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-a", "group-c"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-a", "group-d"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-a", "group-e"}))

	// group-b has shard bcf
	assert.Equal(t, 2, p.EndpointGraph.NumEdges("group-b"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-b", "group-c"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-b", "group-f"}))

	// group-c has shards bcf & cda
	assert.Equal(t, 4, p.EndpointGraph.NumEdges("group-c"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-c", "group-a"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-c", "group-b"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-c", "group-d"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-c", "group-f"}))

	// group-d has shards ade & cda
	assert.Equal(t, 3, p.EndpointGraph.NumEdges("group-d"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-d", "group-a"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-d", "group-c"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-d", "group-e"}))

	// group-e has shard ade
	assert.Equal(t, 2, p.EndpointGraph.NumEdges("group-e"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-e", "group-a"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-e", "group-d"}))

	// group-f has shard bcf
	assert.Equal(t, 2, p.EndpointGraph.NumEdges("group-f"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-f", "group-b"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-f", "group-c"}))

	// Delete shard bcf
	p.ShardDeleteFunc(shuffleShards[0])

	// group-a has shards ade & cda
	assert.Equal(t, 3, p.EndpointGraph.NumEdges("group-a"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-a", "group-c"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-a", "group-d"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-a", "group-e"}))

	// group-b no longer exists
	assert.False(t, p.EndpointGraph.VertexExists("group-b"), "endpoint should not exist")

	// group-c has shard cda
	assert.Equal(t, 2, p.EndpointGraph.NumEdges("group-c"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-c", "group-a"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-c", "group-d"}))

	// group-d has shards ade & cda
	assert.Equal(t, 3, p.EndpointGraph.NumEdges("group-d"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-d", "group-a"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-d", "group-c"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-d", "group-e"}))

	// group-e has shard ade
	assert.Equal(t, 2, p.EndpointGraph.NumEdges("group-e"), "number of edges should match")
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-e", "group-a"}))
	assert.True(t, p.EndpointGraph.Neighbors([]string{"group-e", "group-d"}))

	// group-f no longer exists
	assert.False(t, p.EndpointGraph.VertexExists("group-f"), "endpoint should not exist")

	// Test search of a partial shard using the graph
	assert.False(t, p.ShardExistsWithEndpoints(context.Background(), []string{"group-a", "group-b"}))
	assert.True(t, p.ShardExistsWithEndpoints(context.Background(), []string{"group-a", "group-e"}))
	assert.True(t, p.ShardExistsWithEndpoints(context.Background(), []string{"group-e", "group-a"}))
	assert.True(t, p.ShardExistsWithEndpoints(context.Background(), []string{"group-a", "group-c"}))
	assert.True(t, p.ShardExistsWithEndpoints(context.Background(), []string{"group-c", "group-a"}))
}

func TestChoose(t *testing.T) {
	n := 100
	k := 5
	expected := 75287520

	res, err := controller.Choose(n, k)
	assert.Nil(t, err, "choose should not error")
	assert.Equal(t, *res, expected, "result should match")
}
