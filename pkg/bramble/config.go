package bramble

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type Config struct {
	Module Module `toml:"module"`
}
type Module struct {
	Name string `toml:"name"`
}

func findConfig() (c Config, location string, err error) {
	location, err = os.Getwd()
	if err != nil {
		return
	}
	for {
		var f *os.File
		f, err = os.Open(filepath.Join(location, "bramble.toml"))
		if !os.IsNotExist(err) && err != nil {
			return
		}
		if err == nil {
			_, err = toml.DecodeReader(f, &c)
			return
		}
		if location == filepath.Join(location, "..") {
			err = errors.New("couldn't find a bramble.toml file in this directory or any parent")
			return
		}
		location = filepath.Join(location, "..")
	}
}
