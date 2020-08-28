package derivation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/maxmcd/bramble/pkg/textreplace"
	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// Derivation is the basic building block of a Bramble build
type Derivation struct {
	Name             string
	Outputs          map[string]Output
	Builder          string
	Platform         string
	Args             []string
	Env              map[string]string
	Sources          []string
	InputDerivations []InputDerivation

	// internal fields
	client   *Function
	location string
}

// DerivationOutput tracks the build outputs. Outputs are not included in the
// Derivation hash. The path tracks the output location in the bramble store
// and Dependencies tracks the bramble outputs that are runtime dependencies.
type Output struct {
	Path         string
	Dependencies []string
}

// InputDerivation is one of the derivation inputs. Path is the location of
// the derivation, output is the name of the specific output this derivation
// uses for the build
type InputDerivation struct {
	Path   string
	Output string
}

var (
	_ starlark.Value    = new(Derivation)
	_ starlark.HasAttrs = new(Derivation)
)

func (drv *Derivation) String() string {
	// TODO: we're overriding this for our own purposes. could be confusing
	return fmt.Sprintf("<derivation %q>", drv.Name)
}

func (drv *Derivation) Type() string          { return "derivation" }
func (drv *Derivation) Freeze()               {}
func (drv *Derivation) Truth() starlark.Bool  { return starlark.True }
func (drv *Derivation) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("cmd") }

func (drv *Derivation) Attr(name string) (val starlark.Value, err error) {
	output, ok := drv.Outputs[name]
	if ok {
		return starlark.String(fmt.Sprintf("$bramble_path/%s", output.Path)), nil
	}
	return nil, nil
}

func (drv *Derivation) AttrNames() (out []string) {
	for name := range drv.Outputs {
		out = append(out, name)
	}
	return
}

func (drv *Derivation) prettyJSON() string {
	b, _ := json.MarshalIndent(drv, "", "  ")
	return string(b)
}

func (drv *Derivation) calculateInputDerivations() (err error) {
	// TODO: is this the best way to do this? presumaby in nix it's a language
	// feature

	fileBytes, err := json.Marshal(drv)
	if err != nil {
		return
	}
	for location, derivation := range drv.client.derivations {
		// TODO: check all outputs, not just the default
		if bytes.Contains(fileBytes, []byte(derivation.String())) {
			drv.InputDerivations = append(drv.InputDerivations, InputDerivation{
				Path:   location,
				Output: "out",
			})
		}
	}
	sort.Slice(drv.InputDerivations, func(i, j int) bool {
		id := drv.InputDerivations[i]
		jd := drv.InputDerivations[j]
		return id.Path+id.Output < jd.Path+id.Output
	})
	return nil
}

func (drv *Derivation) computeDerivation() (fileBytes []byte, filename string, err error) {
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

func (drv *Derivation) checkForExisting() (exists bool, err error) {
	_, filename, err := drv.computeDerivation()
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
		return "", errors.Wrap(err, "error calculating relative bramblefile loc")
	}
	runLocation = filepath.Join(destination, relBramblefileLocation)
	if err = os.MkdirAll(runLocation, 0755); err != nil {
		return "", errors.Wrap(err, "error making build directory")
	}
	drv.Env["src"] = destination
	return
}

func (drv *Derivation) writeDerivation() (err error) {
	fileBytes, filename, err := drv.computeDerivation()
	if err != nil {
		return
	}
	fileLocation := drv.client.joinStorePath(filename)

	if !fileExists(fileLocation) {
		return ioutil.WriteFile(fileLocation, fileBytes, 0444)
	}
	return nil
}

func (drv *Derivation) createBuildDir() (tempDir string, err error) {
	return ioutil.TempDir("", TempDirPrefix)
}

func (drv *Derivation) computeOutPath() (outPath string, err error) {
	_, filename, err := drv.computeDerivation()

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
		if v, ok := drv.Env[i]; ok {
			return v
		}
		return ""
	})
}

func (drv *Derivation) build() (err error) {
	buildDir, err := drv.createBuildDir()
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
		url, ok := drv.Env["url"]
		if !ok {
			return errors.New("fetch_url requires the environment variable 'url' to be set")
		}
		hash, ok := drv.Env["hash"]
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
		runLocation, err = drv.assembleSources(buildDir)
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
		for k, v := range drv.Env {
			v = strings.Replace(v, "$bramble_path", drv.client.storePath, -1)
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "out", outPath))
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "bramble_path", drv.client.storePath))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
	}

	matches, hashString, err := drv.hashAndScanDirectory(outPath)
	if err != nil {
		return
	}
	folderName := hashString + "-" + drv.Name
	drv.Outputs["out"] = Output{Path: folderName, Dependencies: matches}

	newPath := drv.client.joinStorePath() + "/" + folderName
	_, doesnotExistErr := os.Stat(newPath)
	drv.client.log.Debug("Output at ", newPath)
	if doesnotExistErr != nil {
		return os.Rename(outPath, newPath)
	}
	// hashed content is already there, just exit
	return
}

func (drv *Derivation) hashAndScanDirectory(location string) (matches []string, hashString string, err error) {
	var storeValues []string
	old := drv.client.storePath
	new := BramblePrefixOfRecord

	for _, derivation := range drv.client.derivations {
		storeValues = append(storeValues, strings.Replace(derivation.String(), "$bramble_path", old, 1))
	}

	errChan := make(chan error)
	resultChan := make(chan map[string]struct{})
	pipeReader, pipeWriter := io.Pipe()

	go func() {
		if err := reptar.Reptar(location, pipeWriter); err != nil {
			errChan <- err
		}
		if err = pipeWriter.Close(); err != nil {
			errChan <- err
		}
	}()
	hasher := NewHasher()
	go func() {
		_, matches, err := textreplace.ReplaceStringsPrefix(pipeReader, hasher, storeValues, old, new)
		if err != nil {
			errChan <- err
		}
		resultChan <- matches
	}()
	select {
	case err := <-errChan:
		return nil, "", err
	case result := <-resultChan:
		for k := range result {
			matches = append(matches, strings.Replace(k, drv.client.storePath, "$bramble_path", 1))
		}
		return matches, hasher.String(), nil
	}
}
