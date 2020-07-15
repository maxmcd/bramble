package bramble

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

type Derivation struct {
	Name        string
	Outputs     map[string]Output
	Builder     string
	Platform    string
	Args        []string
	Environment map[string]string
	Sources     []string

	// internal fields
	client   *Client
	location string
}

var _ starlark.Value = &Derivation{}

func (drv *Derivation) String() string {
	fmt.Println("string requested", drv.Name, drv.Outputs)
	return fmt.Sprintf("$bramble_path/%s", drv.Outputs["out"].Path)
}
func (drv *Derivation) Type() string          { return "Derivation" }
func (drv *Derivation) Freeze()               {}
func (drv *Derivation) Truth() starlark.Bool  { return starlark.True }
func (drv *Derivation) Hash() (uint32, error) { return 0, nil }

func (drv *Derivation) PrettyJSON() string {
	b, _ := json.MarshalIndent(drv, "", "  ")
	return string(b)
}

func hashFile(name string, file io.ReadCloser) (fileHash, filename string, err error) {
	defer file.Close()
	hasher := NewHasher()
	if _, err = hasher.Write([]byte(name)); err != nil {
		return
	}
	if _, err = io.Copy(hasher, file); err != nil {
		return
	}
	filename = fmt.Sprintf("%s-%s", hasher.String(), name)
	return
}

func (drv *Derivation) ComputeDerivation() (fileBytes []byte, filename string, err error) {
	fileBytes, err = json.Marshal(drv)
	if err != nil {
		return
	}
	outputs := drv.Outputs
	// content is hashed without the outputs attribute
	drv.Outputs = nil
	var jsonBytesForHashing []byte
	jsonBytesForHashing, err = json.Marshal(drv)
	if err != nil {
		return
	}
	drv.Outputs = outputs
	fileName := fmt.Sprintf("%s.drv", drv.Name)
	_, filename, err = hashFile(fileName, ioutil.NopCloser(bytes.NewBuffer(jsonBytesForHashing)))
	if err != nil {
		return
	}
	return
}

func (drv *Derivation) CheckForExisting() (exists bool, err error) {
	_, filename, err := drv.ComputeDerivation()
	if err != nil {
		return
	}
	drv.client.log.Debug("derivation " + drv.Name + " evaluates to " + filename)
	existingDrv, exists, err := drv.client.LoadDerivation(filename)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	drv.Outputs = existingDrv.Outputs
	return true, nil
}

func (drv *Derivation) assembleSources(destination string) (runLocation string, err error) {
	if len(drv.Sources) == 0 {
		return
	}
	sources := drv.Sources
	drv.Sources = []string{}
	absDir, err := filepath.Abs(drv.location)
	if err != nil {
		return
	}
	// get absolute paths for all sources
	for i, src := range sources {
		sources[i] = filepath.Join(absDir, src)
	}
	prefix := commonPrefix(append(sources, absDir))

	if err = copyFiles(prefix, sources, destination); err != nil {
		return
	}
	relBramblefileLocation, err := filepath.Rel(prefix, absDir)
	if err != nil {
		return "", errors.Wrap(err, "error calculating relative bramblefild loc")
	}
	runLocation = filepath.Join(destination, relBramblefileLocation)
	if err = os.MkdirAll(runLocation, 0755); err != nil {
		return "", errors.Wrap(err, "error making build directory")
	}
	drv.Environment["src"] = destination
	return
}

func (drv *Derivation) WriteDerivation() (err error) {
	fileBytes, filename, err := drv.ComputeDerivation()
	if err != nil {
		return
	}
	fileLocation := filepath.Join(drv.client.StorePath(), filename)
	_, doesnotExistErr := os.Stat(fileLocation)
	if doesnotExistErr != nil {
		return ioutil.WriteFile(fileLocation, fileBytes, 0444)
	}
	return nil
}

func (drv *Derivation) createTempDir() (tempDir string, err error) {
	return ioutil.TempDir("", TempDirPrefix)
}

func (drv *Derivation) computeOutPath() (outPath string, err error) {
	_, filename, err := drv.ComputeDerivation()

	return filepath.Join(
		drv.client.storePath,
		strings.TrimSuffix(filename, ".drv"),
	), err
}

func (drv *Derivation) expand(s string) string {
	return os.Expand(s, func(i string) string {
		if i == "bramble_path" {
			return drv.client.storePath
		}
		if v, ok := drv.Environment[i]; ok {
			return v
		}
		return ""
	})
}

func (drv *Derivation) Build() (err error) {
	tempDir, err := drv.createTempDir()
	if err != nil {
		return
	}

	outPath, err := drv.computeOutPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(outPath, 0755); err != nil {
		return
	}
	if drv.Builder == "fetch_url" {
		url, ok := drv.Environment["url"]
		if !ok {
			return errors.New("fetch_url requires the environment variable 'url' to be set")
		}
		if drv.client.test {
			url = os.Expand(url, func(i string) string {
				if i == "test_url" {
					return drv.client.testURL
				}
				return ""
			})
		}
		hash, ok := drv.Environment["hash"]
		if !ok {
			return errors.New("fetch_url requires the environment variable 'hash' to be set")
		}
		path, err := drv.client.DownloadFile(url, hash)
		if err != nil {
			return err
		}
		if err = archiver.Unarchive(path, outPath); err != nil {
			return errors.Wrap(err, "error unarchiving")
		}
	} else {
		var runLocation string
		runLocation, err = drv.assembleSources(tempDir)
		if err != nil {
			return
		}

		builderLocation := drv.expand(drv.Builder)

		// TODO: probably just want to expand bramble_path here
		if _, err := os.Stat(builderLocation); err != nil {
			return errors.Wrap(err, "error checking if builder location exists")
		}
		cmd := exec.Command(builderLocation, drv.Args...)
		cmd.Dir = runLocation
		cmd.Env = []string{}
		for k, v := range drv.Environment {
			v = strings.Replace(v, "$bramble_path", drv.client.storePath, -1)
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "out", outPath))
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "bramble_path", drv.client.storePath))
		// cmd.Stdout = os.Stdout
		// cmd.Stderr = os.Stderr
		b, err := cmd.CombinedOutput()
		fmt.Println(string(b))
		if err != nil {
			return err
		}
	}

	hashString := hashDir(outPath)
	folderName := hashString + "-" + drv.Name
	drv.Outputs["out"] = Output{Path: folderName}

	newPath := drv.client.StorePath() + "/" + folderName
	_, doesnotExistErr := os.Stat(newPath)
	drv.client.log.Debug("Output at ", newPath)
	if doesnotExistErr != nil {
		return os.Rename(outPath, newPath)
	}
	// hashed content is already there, just exit
	return
}

type Output struct {
	Path string
}
