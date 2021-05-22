package bramble

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/maxmcd/dag"
)

func TestNewAcyclicGraph(t *testing.T) {
	graph := NewAcyclicGraph()
	seed := DerivationOutput{Filename: "seed", OutputName: "out"}
	make := DerivationOutput{Filename: "make", OutputName: "out"}
	patchelf := DerivationOutput{Filename: "patchelf", OutputName: "out"}
	golang := DerivationOutput{Filename: "golang", OutputName: "out"}
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
	graph.Walk(func(v dag.Vertex) error {
		time.Sleep(time.Millisecond * 100)
		fmt.Println(v, time.Now())
		return nil
	})
}

// quickGraph is used like quickGraph("1-2", "2-3"). All values are
// treated like strings.
func quickGraph(edges ...string) *AcyclicGraph {
	out := NewAcyclicGraph()
	for _, edge := range edges {
		parts := strings.Split(edge, "-")
		if len(parts) != 2 {
			panic("incorrect input: " + edge)
		}
		out.Add(parts[0])
		out.Add(parts[1])
		out.Connect(dag.BasicEdge(parts[0], parts[1]))
	}
	return out
}

func Test_mergeGraphs(t *testing.T) {
	tests := []struct {
		name    string
		args    []*AcyclicGraph
		want    *AcyclicGraph
		wantErr bool
	}{
		{
			name: "connected",
			args: []*AcyclicGraph{
				quickGraph("1-2"),
				quickGraph("2-3"),
			},
			want: quickGraph("1-2", "2-3"),
		},
		{
			name: "disconnected",
			args: []*AcyclicGraph{
				quickGraph("1-2"),
				quickGraph("3-4"),
			},
			// These graphs are disconnected, so a fake root is added
			want: quickGraph("1-2", "3-4", FakeDAGRoot+"-3", FakeDAGRoot+"-1"),
		},
		{
			name: "already using fakeRoot",
			args: []*AcyclicGraph{
				quickGraph("1-2", FakeDAGRoot+"-1"),
				quickGraph("3-4"),
			},
			want: quickGraph("1-2", "3-4", FakeDAGRoot+"-3", FakeDAGRoot+"-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := mergeGraphs(tt.args...)
			if err := graph.Validate(); err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(graph, tt.want) {
				t.Errorf("mergeGraphs() = \n%v, want \n%v", graph, tt.want)
			}
		})
	}
}
