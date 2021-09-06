package bramblebuild

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	ds "github.com/maxmcd/bramble/pkg/data_structures"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
)

var (
	BuildDirPattern       = "bramble_build_directory*" // TODO: does this ensure the same length always?
	PathPaddingCharacters = "bramble_store_padding"
	PathPaddingLength     = 50

	ErrStoreDoesNotExist = errors.New("calculated store path doesn't exist, did the location change?")
)

func NewStore(bramblePath string) (*Store, error) {
	s := &Store{derivations: &DerivationsMap{
		d: map[string]*Derivation{},
	}}
	return s, ensureBramblePath(s, bramblePath)
}

type Store struct {
	BramblePath string
	StorePath   string

	derivations *DerivationsMap
}

func (s *Store) IsEmpty() bool {
	return s.BramblePath == "" || s.StorePath == ""
}

func (s *Store) TempDir() (tempDir string, err error) {
	tempDir, err = ioutil.TempDir(s.StorePath, BuildDirPattern)
	if err != nil {
		return
	}
	return tempDir, os.Chmod(tempDir, 0777)
}

func (s *Store) TempBuildDir() (tempDir string, err error) {
	return ioutil.TempDir(filepath.Join(s.BramblePath, "var/builds"), "build-")
}

func (s *Store) checkForBuiltDerivationOutputs(filename string) (outputs []Output, built bool, err error) {
	existingDrv, err := s.LoadDerivation(filename)
	if err != nil {
		return
	}
	// It's not built if it doesn't exist
	if existingDrv == nil {
		return nil, false, nil
	}
	// It's not built if it doesn't have the outputs we need
	return existingDrv.Outputs, !existingDrv.MissingOutput(), err
}

func (s *Store) LoadDerivation(filename string) (drv *Derivation, err error) {
	defer logger.Debug("loadDerivation ", filename, " ", drv)
	drv = s.derivations.Load(filename)
	if drv != nil && !drv.MissingOutput() {
		// if it has outputs return now
		return
	}
	loc := s.JoinStorePath(filename)
	if !fileutil.FileExists(loc) {
		// If we have the derivation in memory just return it
		if drv != nil {
			return drv, nil
		}
		// Doesn't exist
		return nil, nil
	}
	f, err := os.Open(loc)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer func() { _ = f.Close() }()
	drv = &Derivation{store: s}
	if err = json.NewDecoder(f).Decode(&drv); err != nil {
		return
	}
	s.derivations.Store(filename, drv)
	return drv, nil
}

func ensureBramblePath(s *Store, bramblePath string) (err error) {
	if bramblePath == "" {
		var home string
		home, err = os.UserHomeDir()
		if err != nil {
			return errors.Wrap(err, "error searching for users home directory")
		}
		s.BramblePath = filepath.Join(home, "bramble")
	} else {
		// Ensure we clean the path so that our padding calculation is consistent.
		s.BramblePath = filepath.Clean(bramblePath)
	}

	// No support for relative bramble paths.
	if !filepath.IsAbs(s.BramblePath) {
		return errors.Errorf("bramble path %s must be absolute", s.BramblePath)
	}

	if !fileutil.PathExists(s.BramblePath) {
		// TODO: use logger
		fmt.Println("bramble path directory doesn't exist, creating")
		if err = os.Mkdir(s.BramblePath, 0755); err != nil {
			return err
		}
	}

	fileMap := map[string]struct{}{}
	{
		// List all files in the bramble folder.
		files, err := ioutil.ReadDir(s.BramblePath)
		if err != nil {
			return errors.Wrap(err, "error listing files in the BRAMBLE_PATH")
		}
		for _, file := range files {
			fileMap[file.Name()] = struct{}{}
		}

		// Specifically check for files in the var folder.
		files, _ = ioutil.ReadDir(s.JoinBramblePath("var"))
		if len(files) > 0 {
			for _, file := range files {
				fileMap["var/"+file.Name()] = struct{}{}
			}
		}
	}

	var storeDirectoryName string
	if storeDirectoryName, err = calculatePaddedDirectoryName(s.BramblePath, PathPaddingLength); err != nil {
		return err
	}

	s.StorePath = s.JoinBramblePath(storeDirectoryName)

	// Add store folder with the correct padding and add a convenience symlink
	// in the bramble folder.
	if _, ok := fileMap["store"]; !ok {
		if err = os.MkdirAll(s.StorePath, 0755); err != nil {
			return err
		}
		if err = os.Symlink("."+storeDirectoryName, s.JoinBramblePath("store")); err != nil {
			return err
		}
	}

	folders := []string{
		// TODO: move this to a common cache directory or somewhere else that
		// this would be expected to be
		"tmp", // Tmp folder, probably shouldn't exist.

		"var", // The var folder.

		// Metadata for config files to store recently built derivations so that
		// they're not wiped during GC
		"var/config-registry",

		// Cache for starlark file compilation.
		"var/star-cache",

		// Location to mount chroots for builds
		"var/builds",
	}

	for _, folder := range folders {
		if _, ok := fileMap[folder]; !ok {
			if err = os.Mkdir(s.JoinBramblePath(folder), 0755); err != nil {
				return errors.Wrap(err, fmt.Sprintf("error creating bramble folder %q", folder))
			}
		}
	}

	// otherwise, check if the exact store path we need exists
	if !fileutil.PathExists(s.StorePath) {
		return ErrStoreDoesNotExist
	}

	return
}

func (s *Store) JoinStorePath(v ...string) string {
	return filepath.Join(append([]string{s.StorePath}, v...)...)
}
func (s *Store) JoinBramblePath(v ...string) string {
	return filepath.Join(append([]string{s.BramblePath}, v...)...)
}

func (s *Store) WriteReader(src io.Reader, name string, validateHash string) (contentHash, path string, err error) {
	hshr := hasher.NewHasher()
	file, err := ioutil.TempFile(s.JoinBramblePath("tmp"), "")
	if err != nil {
		err = errors.Wrap(err, "error creating a temporary file for a write to the store")
		return
	}
	tee := io.TeeReader(src, hshr)
	if _, err = io.Copy(file, tee); err != nil {
		err = errors.Wrap(err, "error writing to the temporary store file")
		return
	}
	fileName := hshr.String()
	if validateHash != "" && hshr.Sha256Hex() != validateHash {
		return hshr.Sha256Hex(), "", hasher.ErrHashMismatch
	}
	if name != "" {
		fileName += ("-" + name)
	}
	path = s.JoinStorePath(fileName)
	if err = file.Close(); err != nil {
		return "", "", err
	}
	if er := os.Rename(file.Name(), path); er != nil {
		return "", "", errors.Wrap(er, "error moving file into store")
	}
	if err = os.Chmod(path, 0444); err != nil {
		return
	}

	return hshr.Sha256Hex(), path, nil
}

func (s *Store) CreateTmpFile() (f *os.File, err error) {
	return ioutil.TempFile(s.StorePath, BuildDirPattern)
}

func (s *Store) WriteConfigLink(location string) (err error) {
	hshr := hasher.NewHasher()
	if _, err = hshr.Write([]byte(location)); err != nil {
		return
	}
	reg := s.JoinBramblePath("var/config-registry")
	hash := hshr.String()
	configFileLocation := filepath.Join(reg, hash)
	return ioutil.WriteFile(configFileLocation, []byte(location), 0644)
}

func calculatePaddedDirectoryName(bramblePath string, paddingLength int) (storeDirectoryName string, err error) {
	paddingLen := paddingLength -
		len(bramblePath) - // parent folder lengths
		1 - // slash before directory
		1 // slash after the directory

	if paddingLen <= 0 {
		return "", errors.Errorf(
			"Bramble location creates a path that is too long. "+
				"Location '%s' is too long to create a directory less than %d in length",
			bramblePath, paddingLen)
	}

	paddingSize := len(PathPaddingCharacters)
	repetitions := paddingLen / (paddingSize + 1)
	extra := paddingLen % (paddingSize + 1)

	for i := 0; i < repetitions; i++ {
		storeDirectoryName += ("/" + PathPaddingCharacters)
	}
	storeDirectoryName += ("/" + PathPaddingCharacters[:extra])
	return storeDirectoryName, nil
}

func (s *Store) WriteDerivation(drv *Derivation) error {
	filename := drv.Filename()
	fileLocation := s.JoinStorePath(filename)

	return ioutil.WriteFile(fileLocation, drv.JSON(), 0644)
}

func (s *Store) StoreDerivation(drv *Derivation) {
	s.derivations.Store(drv.Filename(), drv)
}

func (s *Store) stringsReplacerForOutputs(outputs DerivationOutputs) (replacer *strings.Replacer, err error) {
	// Find all the replacements we need to make, template strings need to
	// become filesystem paths
	replacements := []string{}
	for _, do := range outputs {
		d := s.derivations.Load(do.Filename)
		if d == nil {
			return nil, errors.Errorf(
				"couldn't find a derivation with the filename %q in our cache. have we built it yet?", do.Filename)
		}
		path := filepath.Join(
			s.StorePath,
			d.Output(do.OutputName).Path,
		)
		replacements = append(replacements, do.templateString(), path)
	}
	// Replace the content using the json body and then convert it back into a
	// new derivation
	return strings.NewReplacer(replacements...), nil
}

func (s *Store) copyDerivationWithOutputValuesReplaced(drv *Derivation) (copy *Derivation, err error) {
	// Find all derivation output template strings within the derivation
	outputs := drv.InputDerivations

	replacer, err := s.stringsReplacerForOutputs(outputs)
	if err != nil {
		return
	}
	replacedJSON := replacer.Replace(string(drv.JSON()))
	err = json.Unmarshal([]byte(replacedJSON), &copy)
	return copy, err
}

func (s *Store) BuildDerivations(ctx context.Context, derivations []*Derivation, skipDerivation *Derivation) (
	result []BuildResult, err error) {
	// TODO: instead of assembling this graph from dos, generate the dependency
	// graph for each derivation and then just merge the graphs with a fake root
	derivationsMap := DerivationsMap{}
	graphs := []*dag.AcyclicGraph{}
	for _, drv := range derivations {
		derivationsMap.Store(drv.Filename(), drv)
		graph, err := drv.BuildDependencyGraph()
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, graph)
	}
	graph := ds.MergeGraphs(graphs...)
	if graph == nil || len(graph.Vertices()) == 0 {
		return
	}
	if err = graph.Validate(); err != nil {
		err = errors.WithStack(err)
		return
	}
	var wg sync.WaitGroup
	errChan := make(chan error)
	semaphore := make(chan struct{}, 1)
	var errored bool
	if err = graph.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		graph.Walk(func(v dag.Vertex) (_ error) {
			if errored {
				return
			}
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			// serial for now

			// Skip the rake root
			if v == ds.FakeDAGRoot {
				return
			}
			do := v.(DerivationOutput)
			// REFAC: see comment below
			drv := derivationsMap.Load(do.Filename)
			drv.lock.Lock()
			defer drv.lock.Unlock()

			if skipDerivation != nil && skipDerivation == drv {
				// Is this enough of an equality check?
				return
			}
			wg.Add(1)
			// REFAC
			didBuild := true
			// didBuild, err := b.buildDerivationIfNew(ctx, drv)
			// if err != nil {
			// 	// Passing the error might block, so we need an explicit Done
			// 	// call here.
			// 	wg.Done()
			// 	errored = true
			// 	logger.Print(err)
			// 	errChan <- err
			// 	return
			// }

			// Post build processing of dependencies template values:
			{
				// We construct the template value using the DerivationOutput
				// which uses the initial derivation output value
				oldTemplateName := fmt.Sprintf(UnbuiltDerivationOutputTemplate, do.Filename, do.OutputName)

				newTemplateName := drv.OutputTemplateString(do.OutputName)

				for _, edge := range graph.EdgesTo(v) {
					if edge.Source() == ds.FakeDAGRoot {
						continue
					}
					childDO := edge.Source().(DerivationOutput)

					// REFAC: consider moving this cache to just within the builder, can be the context for rebuilding the tree
					childDRV := derivationsMap.Load(childDO.Filename)
					for i, input := range childDRV.InputDerivations {
						// Add the output to the derivation input
						if input.Filename == do.Filename && input.OutputName == do.OutputName {
							childDRV.InputDerivations[i].Output = drv.Output(do.OutputName).Path
						}
					}
					if err := childDRV.replaceValueInDerivation(oldTemplateName, newTemplateName); err != nil {
						panic(err)
					}
				}
			}

			result = append(result, BuildResult{Derivation: drv, DidBuild: didBuild})
			wg.Done()
			return
		})
		errChan <- nil
	}()
	err = <-errChan
	cancel() // Call cancel on the context, no-op if nothing is running
	if err != nil {
		// If we receive an error cancel the context and wait for any jobs that
		// are running.
		wg.Wait()
	}
	return result, err
}
