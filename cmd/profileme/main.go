package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/maxmcd/bramble/pkg/textreplace"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		textreplace.Run()
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
