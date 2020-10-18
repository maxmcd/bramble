package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/containerd/console"
	"github.com/maxmcd/bramble/pkg/progressui"
)

func main() {
	current := console.Current()

	input := make(chan *progressui.SolveStatus)
	go func() {
		n := time.Now()
		input <- &progressui.SolveStatus{
			Vertexes: []*progressui.Vertex{{
				Digest:  "hi",
				Name:    "foo",
				Started: &n,
			}},
			Statuses: []*progressui.VertexStatus{{
				ID:      "yes",
				Vertex:  "hi",
				Name:    "hellooo",
				Total:   123,
				Current: 1,
			}},
			Logs: []*progressui.VertexLog{{
				Vertex:    "hif",
				Data:      []byte("I did!"),
				Timestamp: time.Now(),
			}},
		}
		input <- &progressui.SolveStatus{
			Statuses: []*progressui.VertexStatus{{
				Vertex: "hi",
				Name:   "hellooo",
			}},
			Logs: []*progressui.VertexLog{{
				Vertex:    "hi",
				Data:      []byte("I did!"),
				Timestamp: time.Now(),
			}},
		}
	}()
	log.Fatal(progressui.DisplaySolveStatus(context.Background(), "ok", current, os.Stdout, input))
}
