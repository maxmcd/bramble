package bramble

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform/dag"
)

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
