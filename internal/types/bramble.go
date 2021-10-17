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

type Builder interface {
	Build(ctx context.Context, args []string, opts BuildOptions) (BuildResponse, error)
	Module() (string, string)
}

type NewBuilder func(location string) (Builder, error)
