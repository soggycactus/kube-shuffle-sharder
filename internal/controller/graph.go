package controller

import "sync"

func NewGraph[T comparable]() *Graph[T] {
	return &Graph[T]{
		Vertices: map[T]*Vertex[T]{},
		mu:       new(sync.Mutex),
	}
}

type Graph[T comparable] struct {
	mu       *sync.Mutex
	Vertices map[T]*Vertex[T]
}

type Vertex[T comparable] struct {
	Edges map[T]*Edge[T]
}

type Edge[T comparable] struct {
	Vertex *Vertex[T]
}

func (g *Graph[T]) AddVertexIfNotExists(key T) {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, ok := g.Vertices[key]
	if !ok {
		g.Vertices[key] = &Vertex[T]{
			Edges: make(map[T]*Edge[T]),
		}
	}
}

func (g *Graph[T]) AddEdge(srcKey, destKey T) {
	g.mu.Lock()
	defer g.mu.Unlock()

	src, ok := g.Vertices[srcKey]
	if !ok {
		return
	}

	dst, ok := g.Vertices[destKey]
	if !ok {
		return
	}

	src.Edges[destKey] = &Edge[T]{
		Vertex: dst,
	}
	dst.Edges[srcKey] = &Edge[T]{
		Vertex: src,
	}
}

func (g *Graph[T]) DeleteEdge(srcKey, destKey T) {
	g.mu.Lock()
	defer g.mu.Unlock()

	src, ok := g.Vertices[srcKey]
	if ok {
		delete(src.Edges, destKey)
	}

	dst, ok := g.Vertices[destKey]
	if ok {
		delete(dst.Edges, srcKey)
	}
}

func (g *Graph[T]) PathExists(keys []T) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(keys) == 0 {
		return false
	}

	start, ok := g.Vertices[keys[0]]
	if !ok {
		return false
	}

	for _, key := range keys[1:] {
		if _, ok := start.Edges[key]; !ok {
			return false
		}
	}

	return true
}

func (g *Graph[T]) NumEdges(key T) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return len(g.Vertices[key].Edges)
}

func (g *Graph[T]) DeleteVertex(key T) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.Vertices, key)
}
