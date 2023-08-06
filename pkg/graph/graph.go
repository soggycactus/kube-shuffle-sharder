package graph

import "sync"

func NewGraph[T comparable]() *Graph[T] {
	return &Graph[T]{
		vertices: map[T]*Vertex[T]{},
		mu:       new(sync.Mutex),
	}
}

type Graph[T comparable] struct {
	mu       *sync.Mutex
	vertices map[T]*Vertex[T]
}

type Vertex[T comparable] struct {
	key   T
	edges map[T]*Edge[T]
}

type Edge[T comparable] struct {
	vertex *Vertex[T]
}

func (g *Graph[T]) AddVertexIfNotExists(key T) {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, ok := g.vertices[key]
	if !ok {
		g.vertices[key] = &Vertex[T]{
			key:   key,
			edges: make(map[T]*Edge[T]),
		}
	}
}

func (g *Graph[T]) AddEdge(srcKey, destKey T) {
	g.mu.Lock()
	defer g.mu.Unlock()

	src, ok := g.vertices[srcKey]
	if !ok {
		return
	}

	dst, ok := g.vertices[destKey]
	if !ok {
		return
	}

	src.edges[destKey] = &Edge[T]{
		vertex: dst,
	}
	dst.edges[srcKey] = &Edge[T]{
		vertex: src,
	}
}

func (g *Graph[T]) DeleteEdge(srcKey, destKey T) {
	g.mu.Lock()
	defer g.mu.Unlock()

	src, ok := g.vertices[srcKey]
	if ok {
		delete(src.edges, destKey)
	}

	dst, ok := g.vertices[destKey]
	if ok {
		delete(dst.edges, srcKey)
	}
}

// Neighbors returns true if each key supplied shares an edge with every other key supplied.
// For example, if endpoint "b" has edges with endpoints "c" and "d", but "c" and "d" themselves do not share an edge, this method returns false.
func (g *Graph[T]) Neighbors(keys []T) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(keys) == 0 {
		return false
	}

	for i := 0; i < len(keys); i++ {
		start, ok := g.vertices[keys[i]]
		if !ok {
			return false
		}

		for j := 0; j < len(keys); j++ {
			if i == j {
				continue
			}
			if _, ok := start.edges[keys[j]]; !ok {
				return false
			}
		}
	}

	return true
}

func (g *Graph[T]) NumEdges(key T) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return len(g.vertices[key].edges)
}

func (g *Graph[T]) DeleteVertex(key T) {
	g.mu.Lock()
	defer g.mu.Unlock()

	v, ok := g.vertices[key]
	if !ok {
		return
	}

	// delete all edges of the vertex before deleting the vertex
	for _, edge := range v.edges {
		delete(edge.vertex.edges, key)
		g.vertices[edge.vertex.key] = edge.vertex
	}

	delete(g.vertices, key)
}

func (g *Graph[T]) VertexExists(key T) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, ok := g.vertices[key]
	return ok
}
