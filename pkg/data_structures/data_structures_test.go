package ds

import (
	"reflect"
	"strings"
	"testing"

	"github.com/maxmcd/dag"
)

// quickGraph is used like quickGraph("1-2", "2-3"). All values are treated like
// strings.
func quickGraph(edges ...string) *dag.AcyclicGraph {
	out := &dag.AcyclicGraph{}
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
		args    []*dag.AcyclicGraph
		want    *dag.AcyclicGraph
		wantErr bool
	}{
		{
			name: "connected",
			args: []*dag.AcyclicGraph{
				quickGraph("1-2"),
				quickGraph("2-3"),
			},
			want: quickGraph("1-2", "2-3"),
		},
		{
			name: "disconnected",
			args: []*dag.AcyclicGraph{
				quickGraph("1-2"),
				quickGraph("3-4"),
			},
			// These graphs are disconnected, so a fake root is added
			want: quickGraph("1-2", "3-4", FakeRoot+"-3", FakeRoot+"-1"),
		},
		{
			name: "already using fakeRoot",
			args: []*dag.AcyclicGraph{
				quickGraph("1-2", FakeRoot+"-1"),
				quickGraph("3-4"),
			},
			want: quickGraph("1-2", "3-4", FakeRoot+"-3", FakeRoot+"-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := MergeGraphs(tt.args...)
			if err := graph.Validate(); err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(graph, tt.want) {
				t.Errorf("mergeGraphs() = \n%v, want \n%v", graph, tt.want)
			}
		})
	}
}
