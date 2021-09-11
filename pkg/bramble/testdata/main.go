package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello world")
	exitCode := flag.Int("exit-code", 0, "")
	flag.Parse()
	os.Exit(*exitCode)
}
