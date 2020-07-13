package bramble

import (
	"fmt"
)

var (
	TempDirPrefix = "bramble-"
)

func Run(args []string) (err error) {
	client, err := NewClient()
	if err != nil {
		fmt.Printf("%+v", err)
		return err
	}
	_, err = client.Run(args[1])
	if err != nil {
		fmt.Printf("%+v", err)
	}
	return
}
