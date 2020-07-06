package bramble

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	client *Client
}

var _ starlark.Value = &Derivation{}

func (drv *Derivation) String() string {
	return drv.Outputs["out"].Path
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
	hash := sha256.New()
	if _, err = hash.Write([]byte(name)); err != nil {
		return
	}
	if _, err = io.Copy(hash, file); err != nil {
		return
	}
	var buf bytes.Buffer
	// https://nixos.org/nixos/nix-pills/nix-store-paths.html
	// Finally the comments tell us to compute the base-32 representation of the
	// first 160 bits (truncation) of a sha256 of the above string:
	if _, err = base32.NewEncoder(base32.StdEncoding, &buf).Write(hash.Sum(nil)[:20]); err != nil {
		return
	}

	fileHash = strings.ToLower(buf.String())
	filename = fmt.Sprintf("%s-%s", fileHash, name)
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
	drv.client.log.Debug("derivation '" + drv.Name + " evaluates to " + filename)
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

func (drv *Derivation) AssembleSources(directory string) (err error) {
	if len(drv.Sources) == 0 {
		return nil
	}
	sources := drv.Sources
	drv.Sources = []string{}
	absDir, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	// get absolute paths for all sources
	for i, src := range sources {
		sources[i] = filepath.Join(absDir, src)
	}
	tmpDir, err := ioutil.TempDir("", TempDirPrefix)
	if err != nil {
		return err
	}
	if err = CopyFiles(sources, tmpDir); err != nil {
		return
	}
	hash := hashDir(tmpDir)
	folderName := fmt.Sprintf("%s-source", hash)
	if !Exists(drv.client.StorePath(folderName)) {
		if err = os.Rename(tmpDir, drv.client.StorePath(folderName)); err != nil {
			return
		}
	}
	drv.Environment["src"] = folderName

	return nil
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
	tempDir, err = ioutil.TempDir("", TempDirPrefix)
	if err != nil {
		return
	}
	// TODO: create output folders and environment variables for other outputs
	err = os.MkdirAll(filepath.Join(tempDir, "out"), os.ModePerm)
	return
}

func hashDir(location string) (hash string) {
	shaHash := sha256.New()
	location = filepath.Clean(location) + "/" // use the extra / to make the paths relative

	// TODO: handle common errors like "missing location"
	// likely still want to ignore errors related to missing symlinks, etc...
	// likely with very explicit handling

	// filepath.Walk orders files in lexical order, so this will be deterministic
	_ = filepath.Walk(location, func(path string, info os.FileInfo, _ error) error {
		relativePath := strings.Replace(path, location, "", -1)
		_, _ = shaHash.Write([]byte(relativePath))
		f, err := os.Open(path)
		if err != nil {
			// we already know this file exists, likely just a symlink that points nowhere
			fmt.Println(path, err)
			return nil
		}
		_, _ = io.Copy(shaHash, f)
		f.Close()
		return nil
	})
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(shaHash.Sum(nil)[:20])
	return strings.ToLower(buf.String())
}

func (drv *Derivation) Build() (err error) {
	tempDir, err := drv.createTempDir()
	outPath := filepath.Join(tempDir, "out")
	if err != nil {
		return err
	}
	if drv.Builder == "fetch_url" {
		url, ok := drv.Environment["url"]
		if !ok {
			return errors.New("fetch_url requires the environment variable 'url' to be set")
		}
		var resp *http.Response
		resp, err = http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		drv.client.log.Debugf("Downloading url %s", url)

		hash := sha256.New()
		tee := io.TeeReader(resp.Body, hash)

		var gzReader io.ReadCloser
		gzReader, err = gzip.NewReader(tee)
		// xzReader, err := xz.NewReader(tee)
		if err != nil {
			return err
		}
		defer gzReader.Close()
		if err = Untar(gzReader, outPath); err != nil {
			return
		}
	} else {
		builderLocation := drv.client.StorePath(drv.Builder)
		// TODO: validate this before build?
		if _, err := os.Stat(builderLocation); err != nil {
			return err
		}
		drv.Args[0] = drv.client.StorePath(drv.Environment["src"] + "/simple_builder.sh")
		cmd := exec.Command(builderLocation, drv.Args...)
		cmd.Dir = tempDir
		cmd.Env = []string{}
		for k, v := range drv.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "out", outPath))
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "BRAMBLE_PATH", drv.client.bramblePath))
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
