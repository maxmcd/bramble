package bramble

import (
	"os"
)

var (
	TempDirPrefix = "bramble-"
)

func Run() (err error) {
	client, err := NewClient()
	if err != nil {
		return err
	}
	_, err = client.Run(os.Args[1])
	return
}
