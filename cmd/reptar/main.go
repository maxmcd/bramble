package main

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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

	var fn func(location string, output io.Writer) error

	switch {
	case strings.HasSuffix(args[1], ".tar"):
		fn = reptar.Archive
	case strings.HasSuffix(args[1], ".tar.gz"):
		fn = func(location string, output io.Writer) error {
			zw := gzip.NewWriter(output)
			if err := reptar.Archive(location, zw); err != nil {
				return err
			}
			return zw.Close()
		}
	default:
		return errors.New("archive name must end in .tar or .tar.gz")
	}

	f, err := os.Create(args[1])
	if err != nil {
		return err
	}
	defer f.Close()

	return fn(args[2], f)
}
