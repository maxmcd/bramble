package store

import (
	"context"
	"os"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/pkg/errors"
)

type NewDerivationOptions struct {
	Args             []string
	Builder          string
	Env              map[string]string
	InputDerivations DerivationOutputs
	Name             string
	Outputs          []string
	Platform         string
	Source           Source
	Network          bool
}

type SourceFiles struct {
	ProjectLocation string
	Location        string
	Files           []string
}

func (s *Store) StoreLocalSources(ctx context.Context, sources SourceFiles) (out Source, err error) {
	_, span := tracer.Start(ctx, "store.StoreLocalSources")
	defer span.End()

	if len(sources.Files) == 0 {
		return
	}
	// TODO: could extend reptar to handle hashing the files before moving them
	// to a tempdir
	tmpDir, err := s.storeLengthTempDir()
	if err != nil {
		return
	}

	absDir := filepath.Join(sources.ProjectLocation, sources.Location)

	files := []string{}

	// get absolute paths for all sources
	for _, src := range sources.Files {
		files = append(files, filepath.Join(sources.ProjectLocation, src))
	}

	prefix := fileutil.CommonFilepathPrefix(append(files, absDir))
	relBramblefileLocation, err := filepath.Rel(prefix, absDir)
	if err != nil {
		return
	}

	if err = fileutil.CopyFilesByPath(prefix, files, tmpDir); err != nil {
		err = errors.Wrap(err, "error copying files from source into temp folder")
		return
	}
	// sometimes the location the derivation runs from is not present
	// in the structure of the copied source files. ensure that we add it
	runLocation := filepath.Join(tmpDir, relBramblefileLocation)
	if err = os.MkdirAll(runLocation, 0755); err != nil {
		return
	}
	hshr := hasher.New()
	if err = reptar.Reptar(tmpDir, hshr); err != nil {
		return
	}
	storeLocation := s.joinStorePath(hshr.String())
	if fileutil.PathExists(storeLocation) {
		if err = os.RemoveAll(tmpDir); err != nil {
			return
		}
	} else {
		if err = os.Rename(tmpDir, storeLocation); err != nil {
			return
		}
	}
	out.Path = hshr.String()
	out.RelativeBuildPath = relBramblefileLocation
	return
}

type Source struct {
	RelativeBuildPath string
	Path              string
}

func (s *Store) NewDerivation(options NewDerivationOptions) (exists bool, drv Derivation, err error) {
	drv = s.newDerivation()

	drv.Source = options.Source // TODO: validate this is either empty or the paths are present in the store
	drv.store = s
	drv.Args = options.Args
	drv.Builder = options.Builder
	drv.Network = options.Network
	drv.Name = options.Name
	drv.Env = options.Env
	drv.InputDerivations = options.InputDerivations
	drv.Platform = options.Platform
	drv.OutputNames = options.Outputs // TODO: Validate, and others

	drv = formatDerivation(drv)
	outputs, exists, err := s.checkForBuiltDerivationOutputs(drv)
	drv.Outputs = outputs
	return exists, drv, err
}
