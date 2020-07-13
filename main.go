package main

import (
	"fmt"
	"os"

	"github.com/maxmcd/bramble/pkg/bramble"
	"go.starlark.net/starlark"
)

func main() {
	if err := bramble.Run(os.Args); err != nil {
		valueErr, ok := err.(*starlark.EvalError)
		if ok {
			fmt.Println(valueErr.Backtrace())
		} else {
			fmt.Println(err)
		}
		os.Exit(1)
	}
}
