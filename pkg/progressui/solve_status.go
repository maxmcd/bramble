package progressui

import (
	"time"
)

type SolveStatus struct {
	Vertexes []*Vertex
	Statuses []*VertexStatus
	Logs     []*VertexLog
}

type Vertex struct {
	Digest    Digest
	Inputs    []Digest
	Name      string
	Started   *time.Time
	Completed *time.Time
	Cached    bool
	Error     string
}

type VertexLog struct {
	Vertex    Digest
	Stream    int
	Data      []byte
	Timestamp time.Time
}

type VertexStatus struct {
	ID        string
	Vertex    Digest
	Name      string
	Total     int64
	Current   int64
	Timestamp time.Time
	Started   *time.Time
	Completed *time.Time
}

type Digest string
