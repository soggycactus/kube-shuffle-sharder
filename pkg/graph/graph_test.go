package graph_test

import (
	"testing"

	"github.com/soggycactus/kube-shuffle-sharder/pkg/graph"
	"github.com/stretchr/testify/assert"
)

func TestGraph(t *testing.T) {
	graph := graph.NewGraph[string]()

	graph.AddVertexIfNotExists("a")
	graph.AddVertexIfNotExists("b")
	graph.AddVertexIfNotExists("c")
	graph.AddVertexIfNotExists("d")
	graph.AddVertexIfNotExists("e")

	graph.AddEdge("a", "d")
	graph.AddEdge("a", "b")
	graph.AddEdge("d", "b")
	graph.AddEdge("d", "e")
	graph.AddEdge("b", "e")
	graph.AddEdge("b", "c")
	graph.AddEdge("e", "c")

	assert.Equal(t, graph.NumEdges("a"), 2)
	assert.Equal(t, graph.NumEdges("b"), 4)
	assert.Equal(t, graph.NumEdges("c"), 2)
	assert.Equal(t, graph.NumEdges("d"), 3)
	assert.Equal(t, graph.NumEdges("e"), 3)

	graph.DeleteEdge("a", "d")

	assert.Equal(t, graph.NumEdges("a"), 1)
	assert.Equal(t, graph.NumEdges("b"), 4)
	assert.Equal(t, graph.NumEdges("c"), 2)
	assert.Equal(t, graph.NumEdges("d"), 2)
	assert.Equal(t, graph.NumEdges("e"), 3)

	graph.DeleteVertex("e")

	assert.Equal(t, graph.NumEdges("a"), 1)
	assert.Equal(t, graph.NumEdges("b"), 3)
	assert.Equal(t, graph.NumEdges("c"), 1)
	assert.Equal(t, graph.NumEdges("d"), 1)

	assert.False(t, graph.VertexExists("e"))

	assert.False(t, graph.Neighbors([]string{"b", "c", "d"}))

	graph.AddEdge("a", "d")
	assert.True(t, graph.Neighbors([]string{"a", "d", "b"}))

}
