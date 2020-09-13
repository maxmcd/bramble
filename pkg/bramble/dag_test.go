package bramble

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform/dag"
	"github.com/hashicorp/terraform/tfdiags"
)

func TestNewAcyclicGraph(t *testing.T) {
	graph := NewAcyclicGraph()
	seed := InputDerivation{Path: "seed", Output: "out"}
	make := InputDerivation{Path: "make", Output: "out"}
	patchelf := InputDerivation{Path: "patchelf", Output: "out"}
	golang := InputDerivation{Path: "golang", Output: "out"}
	graph.Add(seed)
	graph.Add(make)
	graph.Add(patchelf)
	graph.Add(golang)
	// graph.Connect(dag.BasicEdge(seed, make))
	// graph.Connect(dag.BasicEdge(seed, patchelf))
	// graph.Connect(dag.BasicEdge(make, patchelf))
	// graph.Connect(dag.BasicEdge(seed, golang))
	// graph.Connect(dag.BasicEdge(make, golang))
	graph.Connect(dag.BasicEdge(make, seed))
	graph.Connect(dag.BasicEdge(patchelf, seed))
	graph.Connect(dag.BasicEdge(patchelf, make))
	graph.Connect(dag.BasicEdge(golang, seed))
	graph.Connect(dag.BasicEdge(golang, make))

	fmt.Println(string(graph.Dot(&dag.DotOpts{Verbose: true, DrawCycles: true})))
	fmt.Println(graph.String())
	for _, e := range graph.EdgesFrom(seed) {
		fmt.Println(e)
	}
	fmt.Println(graph.Validate())
	set := dag.Set{}
	set.Add(golang)
	_ = graph.DepthFirstWalk(set, func(v dag.Vertex, i int) error {
		fmt.Println(v, i)
		return nil
	})
	graph.Walk(func(v dag.Vertex) tfdiags.Diagnostics {
		time.Sleep(time.Millisecond * 100)
		fmt.Println(v, time.Now())
		return nil
	})

}
