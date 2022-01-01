package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maxmcd/bramble/internal/dependency"
	"github.com/maxmcd/bramble/internal/store"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/pkg/errors"
)

type publishOptions struct {
	pkg    string
	upload bool
	local  bool
	url    string
}

func publish(ctx context.Context, opt publishOptions, dgr types.DownloadGithubRepo, cc store.CacheClient) error {
	if opt.local {
		// TODO: add build cache handler to this server
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
		fmt.Println(packages)
		if err != nil {
			return err
		}
		if opt.upload {
			var drvs []store.Derivation
			for _, drvFilename := range builtDerivations {
				drv, _, err := s.LoadDerivation(drvFilename)
				if err != nil {
					return errors.Wrap(err, "error loading derivation from store")
				}
				drvs = append(drvs, drv)
			}
			if cc == nil {
				cc = store.NewS3CacheClient(
					os.Getenv("DIGITALOCEAN_SPACES_ACCESS_ID"),
					os.Getenv("DIGITALOCEAN_SPACES_SECRET_KEY"),
					"nyc3.digitaloceanspaces.com",
				)
			}
			fmt.Printf("Uploading %d derivations\n", len(drvs))
			if err := s.UploadDerivationsToCache(ctx, drvs, cc); err != nil {
				return err
			}

			depManager := dependency.NewManager(filepath.Join(s.BramblePath, "var/dependencies"), "")
			for _, pkg := range packages {
				path := depManager.LocalPackageLocation(pkg)
				fmt.Println("package/source/" + pkg.String())
				writer, err := cc.ObjectWriter(ctx, "package/source/"+pkg.String())
				if err != nil {
					return errors.Wrap(err, "getting object writer")
				}
				if err := reptar.GzipArchive(path, writer); err != nil {
					return errors.Wrap(err, "error archiving object")
				}
				if err := writer.Close(); err != nil {
					return errors.Wrap(err, "error uploading object")
				}
			}

		}
		return nil
	}

	url := "https://store.bramble.run"
	if opt.url != "" {
		url = opt.url
	}
	return dependency.PostJob(ctx, url, opt.pkg, "")

}
