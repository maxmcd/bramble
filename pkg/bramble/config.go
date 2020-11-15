package bramble

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type Config struct {
	Module ConfigModule `toml:"module"`
}
type ConfigModule struct {
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

func (b *Bramble) writeConfigMetadata(derivations []*Derivation) (err error) {
	outputs := []string{}
	for _, drv := range b.inputDerivations {
		outputs = append(outputs, drv.Filename+":"+drv.OutputName)
	}
	for _, drv := range derivations {
		filename := drv.filename()
		// add all outputs for returned derivations
		for _, name := range drv.OutputNames {
			outputs = append(outputs, filename+":"+name)
		}
	}
	derivationsStringMap := map[string][]string{}
	derivationsStringMap[b.moduleEntrypoint+":"+b.calledFunction] = outputs
	if err = b.store.writeConfigLink(b.configLocation, derivationsStringMap); err != nil {
		return
	}
	return nil
}

func (b *Bramble) addURLHashToLockfile(url, hash string) (err error) {
	b.lockFileLock.Lock()
	defer b.lockFileLock.Unlock()

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
	if v, ok := lf.URLHashes[url]; ok && v != hash {
		return errors.Errorf("found existing hash for %q with value %q not %q, not sure how to proceed", url, v, hash)
	}
	lf.URLHashes[url] = hash

	return toml.NewEncoder(f).Encode(&lf)
}
