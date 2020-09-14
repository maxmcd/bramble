package bramble

import (
	"fmt"

	"github.com/hashicorp/terraform/dag"
)

type AcyclicGraph struct {
	dag.AcyclicGraph
}

func NewAcyclicGraph() *AcyclicGraph {
	return &AcyclicGraph{}
}

func (ag AcyclicGraph) PrintDot() {
	fmt.Println(string(ag.Dot(&dag.DotOpts{DrawCycles: true, Verbose: true})))
}
