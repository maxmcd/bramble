package types

import (
	"context"
	"runtime"
)

func Platform() string { return runtime.GOOS + "_" + runtime.GOARCH }

type BuildOptions struct {
	Check bool
}

type BuildResponse struct {
	Modules          map[string]map[string][]string
	FinalHashMapping map[string]string
}

type Module struct {
	Name    string
	Version string
}

type Builder interface {
	Build(ctx context.Context, location string, args []string, opts BuildOptions) (BuildResponse, error)
	Modules() map[string]Module
}

type NewBuilder func(location string) (Builder, error)
