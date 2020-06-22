package main

import (
	"log"

	"github.com/maxmcd/bramble/pkg/bramble"
)

func main() {
	if err := bramble.Run(); err != nil {
		log.Fatal(err)
	}
}
