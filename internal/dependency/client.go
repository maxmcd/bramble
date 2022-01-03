package dependency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/netcache"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/maxmcd/bramble/pkg/reptar"
)

type dependencyClient struct {
	client              *http.Client
	host                string
	cacheClient         netcache.Client
	dependencyDirectory dependencyDirectory
}

func (dc *dependencyClient) request(ctx context.Context, method, path, contentType string, body io.Reader, resp interface{}) (err error) {
	url := fmt.Sprintf("%s/%s",
		strings.TrimSuffix(dc.host, "/"),
		strings.TrimPrefix(path, "/"),
	)
	return httpx.Request(ctx, dc.client, method, url, contentType, body, resp)
}

func (dc *dependencyClient) postJob(ctx context.Context, job JobRequest) (id string, err error) {
	b, err := json.Marshal(job)
	if err != nil {
		return "", err
	}
	return id, dc.request(ctx,
		http.MethodPost,
		"/job",
		"application/json",
		bytes.NewBuffer(b),
		&id)
}

func (dc *dependencyClient) getJob(ctx context.Context, id string) (job Job, err error) {
	return job, dc.request(ctx,
		http.MethodGet,
		"/job/"+id,
		"",
		nil,
		&job)
}

func (dc *dependencyClient) getLogs(ctx context.Context, id string, out io.Writer) (err error) {
	return dc.request(ctx,
		http.MethodGet,
		"/job/"+id+"/logs",
		"",
		nil,
		out)
}

func (dc *dependencyClient) getPackageVersions(ctx context.Context, name string) (vs []string, err error) {
	r, err := dc.cacheClient.Get(ctx, "package/versions/"+name)
	if err != nil {
		return nil, err
	}
	return vs, json.NewDecoder(r).Decode(&vs)
}

func possiblePackageVariants(name string) (variants []string) {
	parts := strings.Split(name, "/")
	for len(parts) > 0 {
		n := strings.Join(parts, "/")
		parts = parts[:len(parts)-1]
		variants = append(variants, n)
	}
	return
}

func (dc *dependencyClient) findPackageFromModuleName(ctx context.Context, name string) (n string, vs []string, err error) {
	for _, n := range possiblePackageVariants(name) {
		vs, err := dc.getPackageVersions(ctx, n)
		if err != nil {
			if err == os.ErrNotExist {
				continue
			}
			return "", nil, err
		}
		return n, vs, nil
	}
	return "", nil, os.ErrNotExist
}

func (dc *dependencyClient) getPackageSource(ctx context.Context, pkg types.Package, location string) (err error) {
	r, err := dc.cacheClient.Get(ctx, "package/source/"+pkg.String())
	if err != nil {
		return err
	}
	return reptar.Unarchive(r, location)
}

func (dc *dependencyClient) getPackageConfig(ctx context.Context, pkg types.Package) (cfg config.ConfigAndLockfile, err error) {
	r, err := dc.cacheClient.Get(ctx, "package/config/"+pkg.String())
	if err != nil {
		return config.ConfigAndLockfile{}, nil
	}
	return cfg, json.NewDecoder(r).Decode(&cfg)
}

func (dc *dependencyClient) uploadPackage(ctx context.Context, pkg types.Package) (err error) {
	location := dc.dependencyDirectory.localPackageLocation(pkg)
	{
		writer, err := dc.cacheClient.Put(ctx, "package/source/"+pkg.String())
		if err != nil {
			return err
		}
		if err := reptar.Archive(location, writer); err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
	}
	{
		cfg, lockfile, err := config.ReadConfigs(location)
		if err != nil {
			return err
		}
		writer, err := dc.cacheClient.Put(ctx, "package/config/"+pkg.String())
		if err != nil {
			return err
		}
		if err := json.NewEncoder(writer).Encode(config.ConfigAndLockfile{Config: cfg, Lockfile: lockfile}); err != nil {
			return err
		}
		return writer.Close()
	}
}
