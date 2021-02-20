package main

import (
	"log"

	"github.com/maxmcd/bramble/pkg/sandbox"
)

func main() {
	if err := sandbox.RunSetUID(); err != nil {
		log.Fatal(err)
	}
}
