package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/maxmcd/bramble/pkg/reptar"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 3 {
		return errors.New("reptar is run like: reptar outputfile.tar.gz ./files-to-package")
	}
	f, err := os.Create(args[1])
	if err != nil {
		return err
	}
	if err = reptar.GzipReptar(args[2], f); err != nil {
		return err
	}
	return f.Close()
}
