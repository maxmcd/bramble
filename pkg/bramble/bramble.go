package bramble

import (
	"fmt"
	"os"
)

var (
	TempDirPrefix = "bramble-"
)

func Run() (err error) {
	client, err := NewClient()
	if err != nil {
		fmt.Printf("%+v", err)
		return err
	}
	_, err = client.Run(os.Args[1])
	if err != nil {
		fmt.Printf("%+v", err)
	}
	return
}
