package controller

func NewGraph() *Graph {
	return &Graph{
		Vertices: map[string]*Vertex{},
	}
}

type Graph struct {
	Vertices map[string]*Vertex
}

type Vertex struct {
	Edges map[string]*Edge
}

type Edge struct {
	Vertex *Vertex
}

func (g *Graph) AddVertexIfNotExists(key string) {
	_, ok := g.Vertices[key]
	if !ok {
		g.Vertices[key] = &Vertex{
			Edges: make(map[string]*Edge),
		}
	}
}

func (g *Graph) AddEdge(srcKey, destKey string) {
	src, ok := g.Vertices[srcKey]
	if !ok {
		return
	}

	dst, ok := g.Vertices[destKey]
	if !ok {
		return
	}

	src.Edges[destKey] = &Edge{
		Vertex: dst,
	}
	dst.Edges[srcKey] = &Edge{
		Vertex: src,
	}
}

func (g *Graph) DeleteEdge(srcKey, destKey string) {
	src, ok := g.Vertices[srcKey]
	if ok {
		delete(src.Edges, destKey)
	}

	dst, ok := g.Vertices[destKey]
	if ok {
		delete(dst.Edges, srcKey)
	}
}
