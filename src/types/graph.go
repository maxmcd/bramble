package types

import (
	"fmt"
	"strings"

	"github.com/maxmcd/dag"
)

// FakeRoot is used when we have multiple build outputs, or "roots" in our
// graph so we need to tie them to a single fake root so that we still have a
// value DAG.
const FakeRoot = "fake root"

// AcyclicGraph
type AcyclicGraph struct {
	dag.AcyclicGraph
}

func NewAcyclicGraph() *AcyclicGraph {
	return &AcyclicGraph{}
}

func PrintDot(ag *dag.AcyclicGraph) {
	fmt.Println(StringDot(ag))
}

func StringDot(ag *dag.AcyclicGraph) string {
	graphString := string(ag.Dot(&dag.DotOpts{DrawCycles: true, Verbose: true}))
	return strings.ReplaceAll(graphString, "\"[root] ", "\"")
}

func MergeGraphs(graphs ...*dag.AcyclicGraph) *dag.AcyclicGraph {
	if len(graphs) == 0 {
		return &dag.AcyclicGraph{}
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
		out.Add(FakeRoot)
		for _, root := range roots {
			if root != FakeRoot {
				out.Connect(dag.BasicEdge(FakeRoot, root))
			}
		}
	}
	return out
}

func graphRoots(g *dag.AcyclicGraph) []dag.Vertex {
	roots := make([]dag.Vertex, 0, 1)
	for _, v := range g.Vertices() {
		if g.UpEdges(v).Len() == 0 {
			roots = append(roots, v)
		}
	}
	return roots
}
