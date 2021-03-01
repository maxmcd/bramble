package main

import (
	"log"

	"github.com/maxmcd/bramble/pkg/sandbox"
)

func main() {
	sandbox.Entrypoint()
	// This shouldn't be reachable unless the entrypoint didn't run
	log.Fatal("can't run bramble-setuid without correct arguments")
}
