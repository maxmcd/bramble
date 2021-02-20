package main

import (
	"log"

	"github.com/maxmcd/bramble/pkg/sandbox"
)

func main() {
	if err := sandbox.RunDebug(); err != nil {
		log.Fatal(err)
	}
}
