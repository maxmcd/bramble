package main

import (
	"fmt"
	"os"

	"github.com/maxmcd/bramble/pkg/bramble"
	"go.starlark.net/starlark"
)

func main() {
	if err := bramble.Run(); err != nil {
		valueErr, ok := err.(*starlark.EvalError)
		if ok {
			fmt.Println(valueErr.Backtrace())
		}
		os.Exit(1)
	}
}
