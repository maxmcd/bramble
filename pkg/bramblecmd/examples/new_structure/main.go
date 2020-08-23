package main

import (
	"fmt"
	"os"

	"github.com/maxmcd/bramble/pkg/bramblecmd"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}
}

func run() (err error) {
	switch os.Args[1] {
	case "run":
		return bramblecmd.Run(os.Args[2])
	case "test":
		return bramblecmd.Test()
	}
	return fmt.Errorf("command %q not found", os.Args[1])
}
