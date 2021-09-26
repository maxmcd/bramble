package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/maxmcd/bramble/pkg/jobprinter"
)

func main() {
	jp := jobprinter.New()

	go func() {
		count := 0
		for {
			job := jp.StartJob(fmt.Sprint(rand.Intn(10)))
			go func() {
				time.Sleep(time.Second * time.Duration(rand.Intn(10)))
				job.ReplaceTS("hi")
				jp.EndJob(job)
			}()
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(900)) * 4)
			count++
			if count > 10 {
				jp.Stop()
			}
		}
	}()

	if err := jp.Start(); err != nil {
		fmt.Println("could not run program:", err)
		os.Exit(1)
	}
}
