package bramblebuild

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/reptar"
)

type NewDerivationOptions struct {
	Args             []string
	Builder          string
	Env              map[string]string
	InputDerivations DerivationOutputs
	Name             string
	Outputs          []string
	Platform         string
	Sources          SourceFiles
}

type SourceFiles struct {
	ProjectLocation string
	Location        string
	Files           []string
}

func (s *Store) hashAndStoreSources(drv *Derivation, sources SourceFiles) (err error) {
	// TODO: could extend reptar to handle hasing the files before moving
	// them to a tempdir
	tmpDir, err := s.TempDir()
	if err != nil {
		return
	}

	absDir, err := filepath.Abs(sources.Location)
	if err != nil {
		return
	}
	// get absolute paths for all sources
	for i, src := range sources.Files {
		sources.Files[i] = filepath.Join(sources.ProjectLocation, src)
	}

	prefix := fileutil.CommonFilepathPrefix(append(sources.Files, absDir))
	relBramblefileLocation, err := filepath.Rel(prefix, absDir)
	if err != nil {
		return
	}

	if err = fileutil.CopyFilesByPath(prefix, sources.Files, tmpDir); err != nil {
		return
	}
	// sometimes the location the derivation runs from is not present
	// in the structure of the copied source files. ensure that we add it
	runLocation := filepath.Join(tmpDir, relBramblefileLocation)
	if err = os.MkdirAll(runLocation, 0755); err != nil {
		return
	}
	hshr := hasher.NewHasher()
	if err = reptar.Reptar(tmpDir, hshr); err != nil {
		return
	}
	storeLocation := s.JoinStorePath(hshr.String())
	if fileutil.PathExists(storeLocation) {
		if err = os.RemoveAll(tmpDir); err != nil {
			return
		}
	} else {
		if err = os.Rename(tmpDir, storeLocation); err != nil {
			return
		}
	}
	drv.BuildContextSource = hshr.String()
	drv.BuildContextRelativePath = relBramblefileLocation
	drv.SourcePaths = append(drv.SourcePaths, hshr.String())
	sort.Strings(drv.SourcePaths)
	return
}

func (s *Store) NewDerivation2(options NewDerivationOptions) (exists bool, drv *Derivation, err error) {
	drv = s.NewDerivation()
	if err = s.hashAndStoreSources(drv, options.Sources); err != nil {
		return
	}
	drv.store = s
	drv.Args = options.Args
	drv.Builder = options.Builder
	drv.Name = options.Name
	drv.Env = options.Env
	drv.InputDerivations = options.InputDerivations
	drv.Platform = options.Platform
	drv.OutputNames = options.Outputs // TODO: Validate, and others

	exists, err = drv.PopulateOutputsFromStore()
	return exists, drv, err
}
