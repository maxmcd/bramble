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
	// Modules is a map of all modules. Each module has a map of all module functions and their outputs.
	Modules map[string]map[string][]string
	// FinalHashMapping is a map of the starlark input hash to the resulting built derivation filename.
	FinalHashMapping map[string]string
}

type Package struct {
	Name    string
	Version string
}

func (p Package) String() string {
	return p.Name + "@" + p.Version
}

type Builder interface {
	// Build runs a build as if it's being run from the command line
	Build(ctx context.Context, location string, args []string, opts BuildOptions) (BuildResponse, error)
	// Packages returns all packages in this location
	Packages() map[string]Package
}

type NewBuilder func(location string) (Builder, error)

type DownloadGithubRepo func(url string, reference string) (location string, err error)
