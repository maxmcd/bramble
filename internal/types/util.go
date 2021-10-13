package types

import (
	"runtime"
)

func Platform() string { return runtime.GOOS + "_" + runtime.GOARCH }
