package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/textreplace"
	"github.com/mholt/archiver/v3"
)

func main() {
	if len(os.Args[1:]) != 4 {
		fmt.Println("should be used like replacer ./folder ./output value1 value2")
		os.Exit(1)
	}
	if err := run(); err != nil {
		panic(err)
	}
}

func run() (err error) {
	location, output, val1, val2 := os.Args[1], os.Args[2], os.Args[3], os.Args[4]

	f, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}
	if err := reptar.Archive(location, f); err != nil {
		return err
	}
	f.Seek(0, 0)

	pipeReader, pipWriter := io.Pipe()
	errChan := make(chan error)
	doneChan := make(chan struct{})

	go func() {
		if _, err := textreplace.ReplaceBytes(
			f, pipWriter,
			[]byte(val1), []byte(val2)); err != nil {
			errChan <- err
			return
		}
		if err := pipWriter.Close(); err != nil {
			errChan <- err
			return
		}
	}()
	go func() {
		tr := archiver.NewTar()
		if err := tr.UnarchiveReader(pipeReader, "", output); err != nil {
			errChan <- err
		}
		doneChan <- struct{}{}
	}()
	select {
	case err := <-errChan:
		return err
	case <-doneChan:
	}

	return nil
}
