package dependency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/maxmcd/bramble/v/cmd/go/mvs"
	"github.com/pkg/errors"
	"golang.org/x/mod/semver"
)

type Manager struct {
	dir              dir
	cfg              config.Config
	dependencyClient *dependencyClient
}

func NewManager(dependencyDir string, cfg config.Config, packageHost string) *Manager {
	return &Manager{
		dir:              dir(dependencyDir),
		cfg:              cfg,
		dependencyClient: &dependencyClient{host: packageHost, client: &http.Client{}},
	}
}

type dir string

func (dd dir) join(v ...string) string {
	return filepath.Join(append([]string{string(dd)}, v...)...)
}

func (dd dir) localModuleVersions(module string) ([]string, error) {
	path := dd.join("src", module)
	searchGlob := fmt.Sprintf("%s*", path)
	matches, err := filepath.Glob(searchGlob)
	if err != nil {
		return nil, err
	}
	for i, match := range matches {
		matches[i] = strings.TrimPrefix(match, path+"@")
	}
	return matches, nil
}

func (dd dir) localModuleLocation(m Version) (path string) {
	return dd.join("src", m.String())
}

func (dm *Manager) ModulePathOrDownload(ctx context.Context, m Version) (path string, err error) {
	path = dm.dir.localModuleLocation(m)
	// If we have it, return it
	if fileutil.DirExists(path) {
		return path, nil
	}
	// If we don't have it, download it
	body, err := dm.dependencyClient.getModuleSource(ctx, m)
	if err != nil {
		if err == os.ErrNotExist {
			return "", errors.Errorf("Module %q doesn't exist in the remote cache, do you need to publish it?", m)
		}
		return "", err
	}
	defer body.Close()
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	// Copy body to file, we can stream the unarchive if we figure out how to
	// get the final size earlier and/or seek over http.
	var name string
	{
		f, err := os.CreateTemp("", "")
		if err != nil {
			return "", err
		}
		name = f.Name()
		_, _ = io.Copy(f, body)
		if err := f.Close(); err != nil {
			return "", err
		}
	}
	if err := chunkedarchive.FileUnarchive(name, path); err != nil {
		return "", errors.Wrap(err, "error unwrapping chunked archive")
	}
	return path, os.RemoveAll(name)
}

type Version struct {
	Module  string
	Version string
}

func (m Version) String() string {
	return m.Module + "@" + m.Version
}
func (m Version) mvsVersion() mvs.Version {
	parts := strings.SplitN(m.Version, ".", 2)
	return mvs.Version{Name: m.Module + "@" + parts[0], Version: parts[1]}
}

func versionFromMVSVersion(m mvs.Version) Version {
	loc := strings.LastIndex(m.Name, "@")
	return Version{Version: m.Name[loc+1:] + "." + m.Version, Module: m.Name[:loc]}
}

func sortVersions(vs []Version) {
	sort.Slice(vs, func(i, j int) bool { return vs[i].Module < vs[j].Module })
}

func configVersions(cfg config.Config) (vs []Version) {
	for module, dep := range cfg.Dependencies {
		vs = append(vs, Version{Module: module, Version: dep.Version})
	}
	sortVersions(vs)
	return vs
}

func (dm *Manager) existsLocally(m Version) bool {
	return fileutil.PathExists(dm.dir.localModuleLocation(m))
}

func (dm *Manager) localModuleDependencies(m Version) (vs []Version, err error) {
	cfg, err := config.ReadConfig(filepath.Join(dm.dir.localModuleLocation(m), "bramble.toml"))
	if err != nil {
		return nil, err
	}
	return configVersions(cfg), nil
}

func (dm *Manager) remoteModuleDependencies(ctx context.Context, m Version) (vs []Version, err error) {
	cfg, err := dm.dependencyClient.getModuleConfig(ctx, m)
	if err != nil {
		return nil, err
	}
	return configVersions(cfg), nil
}

func PostJob(url, module, reference string) (err error) {
	jr := JobRequest{Module: module, Reference: reference}
	dc := &dependencyClient{client: &http.Client{}, host: url}
	id, err := dc.postJob(context.Background(), jr)
	if err != nil {
		return err
	}
	for {
		job, err := dc.getJob(context.Background(), id)
		spew.Dump(job)
		if err != nil {
			return err
		}
		if job.Error != "" {
			return errors.Wrap(errors.New(job.Error), "got error posting job")
		}
		if !job.End.IsZero() {
			break
		}
		time.Sleep(time.Second)
	}
	return nil
}

func (dm *Manager) reqs() mvs.Reqs {
	return dependencyManagerReqs{deps: dm}
}

type dependencyManagerReqs struct {
	deps *Manager
}

var _ mvs.Reqs = dependencyManagerReqs{}

func (r dependencyManagerReqs) Required(m mvs.Version) (versions []mvs.Version, err error) {
	v := versionFromMVSVersion(m)
	var vs []Version

	switch {
	case r.deps.cfg.Module.Name == v.Module && r.deps.cfg.Module.Version == v.Version:
		for module, cd := range r.deps.cfg.Dependencies {
			vs = append(vs, Version{Module: module, Version: cd.Version})
			sortVersions(vs)
		}
	case r.deps.existsLocally(v):
		vs, err = r.deps.localModuleDependencies(v)
	default:
		// TODO: tracing
		// TODO: cache this result locally?
		vs, err = r.deps.remoteModuleDependencies(context.Background(), v)
	}
	if err != nil {
		return nil, err
	}
	for _, v := range vs {
		versions = append(versions, v.mvsVersion())
	}
	return
}
func (r dependencyManagerReqs) Max(v1, v2 string) (o string) {
	switch semver.Compare("v0."+v1, "v0."+v2) {
	case -1:
		return v2
	default:
		return v1
	}
}
func (r dependencyManagerReqs) Upgrade(m mvs.Version) (v mvs.Version, err error) {
	panic("")
	return
}
func (r dependencyManagerReqs) Previous(m mvs.Version) (v mvs.Version, err error) {
	panic("")
	return
}

type dependencyClient struct {
	client *http.Client
	host   string
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

func (dc *dependencyClient) getModuleVersions(ctx context.Context, name string) (vs []string, err error) {
	return vs, dc.request(ctx,
		http.MethodGet,
		"/module/"+name,
		"",
		nil,
		&vs)
}

func (dc *dependencyClient) getModuleSource(ctx context.Context, m Version) (body io.ReadCloser, err error) {
	return body, dc.request(ctx,
		http.MethodGet,
		"/module/source/"+m.String(),
		"",
		nil, &body)
}

func (dc *dependencyClient) getModuleConfig(ctx context.Context, m Version) (cfg config.Config, err error) {
	var buf bytes.Buffer
	var w io.Writer = &buf
	if err := dc.request(ctx,
		http.MethodGet,
		"/module/config/"+m.String(),
		"",
		nil, w); err != nil {
		return cfg, err
	}
	return config.ParseConfig(&buf)
}

func addDependencyMetadata(dependencyDir, module, version, src string, mapping map[string]map[string][]string) (err error) {
	srcs := filepath.Join(dependencyDir, "src")
	fileDest := filepath.Join(srcs, module+"@"+version)

	// TODO, should be platform specific
	drvs := filepath.Join(dependencyDir, ""+types.Platform())
	metadataDest := filepath.Join(drvs, module+"@"+version)

	// If the metadata is here we already have a record of the output mapping.
	// If we checked the src directory it might just be there as a dependency of
	// another nomad project
	if fileutil.PathExists(metadataDest) {
		return errors.Errorf("version %s of module %q is already present on this server", version, module)
	}

	if err := os.MkdirAll(fileDest, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(metadataDest), 0755); err != nil {
		return err
	}
	if err := fileutil.CopyDirectory(src, fileDest); err != nil {
		return err
	}

	f, err := os.Create(metadataDest)
	if err != nil {
		return err
	}
	e := json.NewEncoder(f)
	e.SetIndent("", "  ")
	if err := e.Encode(mapping); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func serverHandler(dependencyDir string, newBuilder types.NewBuilder, downloadGithubRepo func(url string, reference string) (location string, err error)) http.Handler {
	dependencyDirectory := dir(dependencyDir)

	router := httpx.New()
	router.GET("/job/:id", func(c httpx.Context) error {
		job := jq.Lookup(c.Params.ByName("id"))
		if job == nil {
			return httpx.ErrNotFound(errors.New("no job found with that id"))
		}
		return json.NewEncoder(c.ResponseWriter).Encode(job)
	})
	router.POST("/job", func(c httpx.Context) error {
		jobRequest := JobRequest{}
		if err := json.NewDecoder(c.Request.Body).Decode(&jobRequest); err != nil {
			return httpx.ErrUnprocessableEntity(err)
		}
		job := &Job{
			Module:    jobRequest.Module,
			Reference: jobRequest.Reference,
		}
		jq.AddJob(job)
		fmt.Fprint(c.ResponseWriter, job.ID)

		// Run job
		go func() {
			jq.End(job.ID, buildJob(job, dependencyDir, newBuilder, downloadGithubRepo))

		}()

		return nil
	})
	// router.GET("/module/outputs/:platform/:name/:version", func(c httpx.Context) error {
	// 	name := c.Params.ByName("name")
	// 	path := filepath.Join(bramblePath, "var", platform, name)
	// 	searchGlob := fmt.Sprintf("%s*", path)
	// 	matches, err := filepath.Glob(searchGlob)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	for i, match := range matches {
	// 		matches[i] = strings.TrimPrefix(match, path+"@")
	// 	}
	// 	return json.NewEncoder(c.ResponseWriter).Encode(matches)
	// })
	// This is hard because :name can have slashes...
	// router.GET("/module/platform/:platform/:name_version/", func(c httpx.Context) error { return nil })
	router.GET("/module/versions/*name", func(c httpx.Context) error {
		// TODO: Return all matches for cached derivation outputs that we have
		// as well?
		name := c.Params.ByName("name")
		matches, err := dependencyDirectory.localModuleVersions(name)
		if err != nil {
			return err
		}
		return json.NewEncoder(c.ResponseWriter).Encode(matches)
	})
	router.GET("/module/source/*name_version", func(c httpx.Context) error {
		name := c.Params.ByName("name_version")
		path := filepath.Join(dependencyDir, "src", name)
		if !fileutil.DirExists(path) {
			return httpx.ErrNotFound(errors.New("can't find module"))
		}
		return chunkedarchive.StreamArchive(c.ResponseWriter, path)
	})
	router.GET("/module/config/*name_version", func(c httpx.Context) error {
		name := c.Params.ByName("name_version")
		path := filepath.Join(dependencyDir, "src", name, "bramble.toml")
		if !fileutil.FileExists(path) {
			return httpx.ErrNotFound(errors.New("can't find module"))
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(c.ResponseWriter, f); err != nil {
			return err
		}
		return nil
	})

	return router
}

func buildJob(job *Job, dependencyDir string, newBuilder types.NewBuilder, downloadGithubRepo func(url string, reference string) (location string, err error)) (err error) {
	loc, err := downloadGithubRepo(job.Module, job.Reference)
	if err != nil {
		return errors.Wrap(err, "error downloading git repo")
	}
	builder, err := newBuilder(loc)
	if err != nil {
		return
	}
	modules := builder.Modules()
	for path, module := range modules {
		rel, err := filepath.Rel(loc, path)
		if err != nil {
			panic(loc + " - " + path)
		}
		expectedModuleName := strings.TrimSuffix(job.Module+"/"+strings.Trim(strings.TrimPrefix(rel, "."), "/"), "/")
		if expectedModuleName != module.Name {
			return errors.Errorf("Project module name %q does not match the location the project was fetched from: %q",
				module.Name,
				expectedModuleName)
		}
	}
	toRun := []func() error{}
	for path, module := range modules {
		resp, err := builder.Build(context.Background(), path, nil, types.BuildOptions{Check: true})
		if err != nil {
			return err
		}
		// Only add if we return without erroring
		toRun = append(toRun, func() error {
			return addDependencyMetadata(
				dependencyDir,
				module.Name,
				module.Version,
				loc,
				resp.Modules)
		})
	}
	for _, tr := range toRun {
		if err = tr(); err != nil {
			return err
		}
	}
	return nil
}

func ServerHandler(dependencyDir string, newBuilder types.NewBuilder) http.Handler {
	return serverHandler(dependencyDir, newBuilder, downloadGithubRepo)
}

func downloadGithubRepo(url string, reference string) (location string, err error) {
	url = "https://" + url + ".git"
	location, err = os.MkdirTemp("", "")
	if err != nil {
		return
	}

	// TODO: replace all this with calls to bramble
	script := fmt.Sprintf(`
	set -ex
	git clone %s %s
	cd %s`, url, location, location)
	if reference != "" {
		script += fmt.Sprintf("\ngit checkout %s", reference)
	}
	// script += "\nrm -rf ./.git"
	cmd := exec.Command("bash", "-c", script)
	var buf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return "", errors.Wrap(err, buf.String())
	}
	return
}
