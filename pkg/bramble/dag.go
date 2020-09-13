package bramble

import (
	"github.com/hashicorp/terraform/dag"
)

type AcyclicGraph struct {
	dag.AcyclicGraph
}

func NewAcyclicGraph() *AcyclicGraph {
	return &AcyclicGraph{}
}
