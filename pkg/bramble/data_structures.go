package bramble

import (
	"fmt"
	"strings"
	"sync"

	"github.com/maxmcd/dag"
)

type DerivationsMap struct {
	d    map[string]*Derivation
	lock sync.RWMutex
}

func (dm *DerivationsMap) Load(filename string) *Derivation {
	dm.lock.RLock()
	defer dm.lock.RUnlock()
	return dm.d[filename]
}

func (dm *DerivationsMap) Has(filename string) bool {
	return dm.Load(filename) != nil
}
func (dm *DerivationsMap) Store(filename string, drv *Derivation) {
	dm.lock.Lock()
	defer dm.lock.Unlock()
	dm.d[filename] = drv
}

func (dm *DerivationsMap) Range(cb func(map[string]*Derivation)) {
	dm.lock.Lock()
	cb(dm.d)
	dm.lock.Unlock()
}

// FakeDAGRoot is used when we have multiple build outputs, or "roots" in our
// graph so we need to tie them to a single fake root so that we still have a
// value DAG.
const FakeDAGRoot = "fakeDAGRoot"

// AcyclicGraph
type AcyclicGraph struct {
	dag.AcyclicGraph
}

func NewAcyclicGraph() *AcyclicGraph {
	return &AcyclicGraph{}
}

func (ag AcyclicGraph) PrintDot() {
	graphString := string(ag.Dot(&dag.DotOpts{DrawCycles: true, Verbose: true}))
	fmt.Println(strings.ReplaceAll(graphString, "\"[root] ", "\""))
}

func mergeGraphs(graphs ...*AcyclicGraph) *AcyclicGraph {
	if len(graphs) == 0 {
		return NewAcyclicGraph()
	}
	if len(graphs) == 1 {
		return graphs[0]
	}
	out := graphs[0]
	for _, graph := range graphs[1:] {
		// Add all vertices and edges to the output graph
		for _, vertex := range graph.Vertices() {
			out.Add(vertex)
		}
		for _, edge := range graph.Edges() {
			out.Connect(edge)
		}
	}

	roots := graphRoots(out)
	if len(roots) != 1 {
		out.Add(FakeDAGRoot)
		for _, root := range roots {
			if root != FakeDAGRoot {
				out.Connect(dag.BasicEdge(FakeDAGRoot, root))
			}
		}
	}
	return out
}

func graphRoots(g *AcyclicGraph) []dag.Vertex {
	roots := make([]dag.Vertex, 0, 1)
	for _, v := range g.Vertices() {
		if g.UpEdges(v).Len() == 0 {
			roots = append(roots, v)
		}
	}
	return roots
}

type BiStringMap struct {
	s       sync.RWMutex
	forward map[string]string
	inverse map[string]string
}

// NewBiStringMap returns a an empty, mutable, BiStringMap
func NewBiStringMap() *BiStringMap {
	return &BiStringMap{
		forward: make(map[string]string),
		inverse: make(map[string]string),
	}
}

func (b *BiStringMap) Store(k, v string) {
	b.s.Lock()
	b.forward[k] = v
	b.inverse[v] = k
	b.s.Unlock()
}

func (b *BiStringMap) Load(k string) (v string, exists bool) {
	b.s.RLock()
	v, exists = b.forward[k]
	b.s.RUnlock()
	return
}

func (b *BiStringMap) StoreInverse(k, v string) {
	b.s.Lock()
	b.forward[v] = k
	b.inverse[k] = v
	b.s.Unlock()
}

func (b *BiStringMap) LoadInverse(k string) (v string, exists bool) {
	b.s.RLock()
	v, exists = b.inverse[k]
	b.s.RUnlock()
	return
}
