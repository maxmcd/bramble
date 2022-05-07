package command

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/maxmcd/bramble/internal/dependency"
	"github.com/maxmcd/bramble/internal/netcache"
	"github.com/maxmcd/bramble/internal/store"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/pkg/errors"
)

type publishOptions struct {
	pkg    string
	upload bool
	local  bool
	url    string
}

func publish(ctx context.Context, opt publishOptions, dgr types.DownloadGithubRepo, cacheClient netcache.Client) error {
	cc := store.NewCacheClient(cacheClient)

	// Regular behavior, publish the job to the CDN
	if !opt.local {
		url := "https://store.bramble.run"
		if opt.url != "" {
			url = opt.url
		}
		return dependency.PostJob(ctx, url, opt.pkg, "")
	}
	fmt.Println("Building package locally")
	// Pull down and build a package locally
	s, err := store.NewStore("")
	if err != nil {
		return err
	}
	builder := dependency.Builder(
		filepath.Join(s.BramblePath, "var/dependencies"),
		newBuilder(s),
		dgr,
	)

	builtDerivations, packages, err := builder(ctx, opt.pkg)
	if err != nil {
		return err
	}
	if !opt.upload {
		return nil
	}
	var drvs []store.Derivation
	for _, drvFilename := range builtDerivations {
		drv, _, err := s.LoadDerivation(drvFilename)
		if err != nil {
			return errors.Wrap(err, "error loading derivation from store")
		}
		drvs = append(drvs, drv)
	}
	fmt.Printf("Uploading %d derivations\n", len(drvs))
	if err := s.UploadDerivationsToCache(ctx, drvs, cc); err != nil {
		return err
	}
	fmt.Println("Uploading packages")
	depManager := dependency.NewManager(
		filepath.Join(s.BramblePath, "var/dependencies"), "",
		cacheClient,
	)
	for _, pkg := range packages {
		if err := depManager.UploadPackage(ctx, pkg); err != nil {
			return err
		}
	}
	return nil
}
