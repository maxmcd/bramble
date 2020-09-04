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

func findConfig() (c Config, lf LockFile, location string, err error) {
	location, err = os.Getwd()
	if err != nil {
		return
	}
	var f *os.File
	for {
		f, err = os.Open(filepath.Join(location, "bramble.toml"))
		if !os.IsNotExist(err) && err != nil {
			return
		}
		if err == nil {
			_, err = toml.DecodeReader(f, &c)
			_ = f.Close()
			break
		}
		if location == filepath.Join(location, "..") {
			err = errors.New("couldn't find a bramble.toml file in this directory or any parent")
			return
		}
		_ = f.Close()
		location = filepath.Join(location, "..")
	}
	if err != nil {
		return
	}
	lockFile := filepath.Join(location, "bramble.lock")
	if fileExists(lockFile) {
		f, err = os.Open(lockFile)
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()
		_, err = toml.DecodeReader(f, &lf)
	}
	return
}

type LockFile struct {
	URLHashes map[string]string
}

func (b *Bramble) writeLockfileAndMetadata(derivations []*Derivation) (err error) {
	outputs := []string{}
	for _, drv := range b.cmd.inputDerivations {
		outputs = append(outputs, drv.Path+":"+drv.Output)
	}
	for _, drv := range derivations {
		// add all outputs for returned derivations
		for name, out := range drv.Outputs {
			outputs = append(outputs, out.Path+":"+name)
		}
	}
	derivationsStringMap := map[string][]string{}
	derivationsStringMap[b.moduleEntrypoint+":"+b.calledFunction] = outputs
	if err = b.store.writeConfigLink(b.configLocation, derivationsStringMap); err != nil {
		return
	}

	f, err := os.OpenFile(filepath.Join(b.configLocation, "bramble.lock"),
		os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	lf := LockFile{
		URLHashes: map[string]string{},
	}
	if _, err = toml.DecodeReader(f, &lf); err != nil {
		return
	}
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	return toml.NewEncoder(f).Encode(&lf)
}
