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
	dir dir

	dependencyClient *dependencyClient
}

func NewManager(dependencyDir string, packageHost string) *Manager {
	return &Manager{
		dir:              dir(dependencyDir),
		dependencyClient: &dependencyClient{host: packageHost, client: &http.Client{}},
	}
}

type dir string

func (dd dir) join(v ...string) string {
	return filepath.Join(append([]string{string(dd)}, v...)...)
}

func (dd dir) localPackageVersions(pkg string) ([]string, error) {
	path := dd.join("src", pkg)
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

func (dd dir) localPackageLocation(pkg types.Package) (path string) {
	return dd.join("src", pkg.String())
}

func (dm *Manager) PackagePathOrDownload(ctx context.Context, pkg types.Package) (path string, err error) {
	path = dm.dir.localPackageLocation(pkg)
	if fileutil.DirExists(path) {
		return path, nil
	}
	body, err := dm.dependencyClient.getPackageSource(ctx, pkg)
	if err != nil {
		if err == os.ErrNotExist {
			return "", errors.Errorf("Package %q doesn't exist in the remote cache, do you need to publish it?", pkg)
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

func (dm *Manager) FindPackage(name string) {

}

func mvsVersionFromPackage(p types.Package) mvs.Version {
	parts := strings.SplitN(p.Version, ".", 2)
	return mvs.Version{Name: p.Name + "@" + parts[0], Version: parts[1]}
}

func packageFromMVSVersion(m mvs.Version) types.Package {
	loc := strings.LastIndex(m.Name, "@")
	return types.Package{Name: m.Name[:loc], Version: m.Name[loc+1:] + "." + m.Version}
}

func sortVersions(pkgs []types.Package) {
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })
}

func configVersions(cfg config.Config) (pkgs []types.Package) {
	for pkg, dep := range cfg.Dependencies {
		pkgs = append(pkgs, types.Package{Name: pkg, Version: dep.Version})
	}
	sortVersions(pkgs)
	return pkgs
}

func (dm *Manager) existsLocally(pkg types.Package) bool {
	return fileutil.PathExists(dm.dir.localPackageLocation(pkg))
}

func (dm *Manager) localPackageDependencies(pkg types.Package) (vs []types.Package, err error) {
	cfg, err := config.ReadConfig(filepath.Join(dm.dir.localPackageLocation(pkg), "bramble.toml"))
	if err != nil {
		return nil, err
	}
	return configVersions(cfg), nil
}

func (dm *Manager) CalculateConfigBuildlist(cfg config.Config) (config.Config, error) {
	versions, err := mvs.BuildList(
		mvsVersionFromPackage(types.Package{Name: cfg.Package.Name, Version: cfg.Package.Version}),
		dm.reqs(cfg),
	)
	if err != nil {
		return config.Config{}, err
	}

	cfg.Dependencies = make(map[string]config.Dependency)
	for _, version := range versions {
		v := packageFromMVSVersion(version)
		if v.Name == cfg.Package.Name {
			continue
		}
		// Support path overrides
		cfg.Dependencies[v.Name] = config.Dependency{Version: v.Version}
	}
	return cfg, nil
}

func (dm *Manager) remotePackageDependencies(ctx context.Context, m types.Package) (vs []types.Package, err error) {
	cfg, err := dm.dependencyClient.getPackageConfig(ctx, m)
	if err != nil {
		return nil, err
	}
	return configVersions(cfg), nil
}

func PostJob(url, pkg, reference string) (err error) {
	jr := JobRequest{Package: pkg, Reference: reference}
	dc := &dependencyClient{client: &http.Client{}, host: url}
	id, err := dc.postJob(context.Background(), jr)
	if err != nil {
		return err
	}
	dur := (time.Millisecond * 100)
	count := 0
	for {
		if count > 5 {
			// Many jobs finish quickly, but if they don't, we can check less
			// often
			dur = time.Second
		}
		job, err := dc.getJob(context.Background(), id)
		spew.Dump(job)
		if err != nil {
			return err
		}
		if job.Error != "" {
			return errors.Wrap(errors.New(job.ErrWithStack), "got error posting job")
		}
		if !job.End.IsZero() {
			break
		}
		count++
		time.Sleep(dur)
	}
	return nil
}

func (dm *Manager) reqs(cfg config.Config) mvs.Reqs {
	return dependencyManagerReqs{deps: dm, cfg: cfg}
}

type dependencyManagerReqs struct {
	deps *Manager
	cfg  config.Config
}

var _ mvs.Reqs = dependencyManagerReqs{}

func (r dependencyManagerReqs) Required(m mvs.Version) (versions []mvs.Version, err error) {
	p := packageFromMVSVersion(m)
	var pkgs []types.Package

	switch {
	case r.cfg.Package.Name == p.Name && r.cfg.Package.Version == p.Version:
		for pkg, cd := range r.cfg.Dependencies {
			pkgs = append(pkgs, types.Package{Name: pkg, Version: cd.Version})
			sortVersions(pkgs)
		}
	case r.deps.existsLocally(p):
		pkgs, err = r.deps.localPackageDependencies(p)
	default:
		// TODO: tracing
		// TODO: cache this result locally?
		pkgs, err = r.deps.remotePackageDependencies(context.Background(), p)
	}
	if err != nil {
		return nil, errors.Wrap(err, "error fetching package")
	}
	for _, p := range pkgs {
		versions = append(versions, mvsVersionFromPackage(p))
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

func (dc *dependencyClient) getPackageVersions(ctx context.Context, name string) (vs []string, err error) {
	return vs, dc.request(ctx,
		http.MethodGet,
		"/package/"+name,
		"",
		nil,
		&vs)
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

func (dd dir) findPackageFromModuleName(module string) (name string, vs []string, err error) {
	for _, n := range possiblePackageVariants(module) {
		vs, err := dd.localPackageVersions(n)
		if err != nil {
			return "", nil, err
		}
		// Found something
		if len(vs) > 0 {
			return n, vs, nil
		}
	}
	return "", nil, os.ErrNotExist
}
func (dm *Manager) FindPackageFromModuleName(ctx context.Context, module string) (name string, vs []string, err error) {
	// Prefer local
	name, vs, err = dm.dir.findPackageFromModuleName(module)
	if err != nil && err != os.ErrNotExist {
		return "", nil, err
	}
	if err == os.ErrNotExist {
		name, vs, err = dm.dependencyClient.findPackageFromModuleName(ctx, module)
	}
	if err == os.ErrNotExist {
		return "", nil, errors.Errorf("can't find package for module %q", module)
	}
	return name, vs, err
}

func (dc *dependencyClient) getPackageSource(ctx context.Context, pkg types.Package) (body io.ReadCloser, err error) {
	if err := dc.request(ctx,
		http.MethodGet,
		"/package/source/"+pkg.String(),
		"",
		nil, &body); err != nil {
		if err == os.ErrNotExist {
			err = errors.Errorf("request to server could not find package %s", pkg)
		}
		return nil, err
	}
	return body, nil
}

func (dc *dependencyClient) getPackageConfig(ctx context.Context, pkg types.Package) (cfg config.Config, err error) {
	var buf bytes.Buffer
	var w io.Writer = &buf
	if err := dc.request(ctx,
		http.MethodGet,
		"/package/config/"+pkg.String(),
		"",
		nil, w); err != nil {
		if err == os.ErrNotExist {
			err = errors.Errorf("request to server could not find package %s", pkg)
		}
		return cfg, err
	}
	return config.ParseConfig(&buf)
}

func addDependencyMetadata(dependencyDir, pkg, version, src string, mapping map[string]map[string][]string) (err error) {
	srcs := filepath.Join(dependencyDir, "src")
	fileDest := filepath.Join(srcs, pkg+"@"+version)

	// TODO, should be platform specific
	drvs := filepath.Join(dependencyDir, ""+types.Platform())
	metadataDest := filepath.Join(drvs, pkg+"@"+version)

	// If the metadata is here we already have a record of the output mapping.
	// If we checked the src directory it might just be there as a dependency of
	// another nomad project
	fmt.Println(metadataDest)
	if fileutil.PathExists(metadataDest) {
		return errors.Errorf("version %s of package %q is already present on this server", version, pkg)
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
			Package:   jobRequest.Package,
			Reference: jobRequest.Reference,
		}
		jq.AddJob(job)
		fmt.Fprint(c.ResponseWriter, job.ID)

		// Run job
		go func() {
			_, err := buildJob(job, dependencyDir, newBuilder, downloadGithubRepo)
			jq.End(job.ID, err)
		}()

		return nil
	})
	// router.GET("/package/outputs/:platform/:name/:version", func(c httpx.Context) error {
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
	// router.GET("/package/platform/:platform/:name_version/", func(c httpx.Context) error { return nil })
	router.GET("/package/versions/*name", func(c httpx.Context) error {
		// TODO: Return all matches for cached derivation outputs that we have
		// as well?
		name := c.Params.ByName("name")
		matches, err := dependencyDirectory.localPackageVersions(name)
		if err != nil {
			return err
		}
		return json.NewEncoder(c.ResponseWriter).Encode(matches)
	})
	router.GET("/package/source/*name_version", func(c httpx.Context) error {
		name := c.Params.ByName("name_version")
		path := filepath.Join(dependencyDir, "src", name)
		if !fileutil.DirExists(path) {
			return httpx.ErrNotFound(errors.New("can't find package"))
		}
		return chunkedarchive.StreamArchive(c.ResponseWriter, path)
	})
	router.GET("/package/config/*name_version", func(c httpx.Context) error {
		name := c.Params.ByName("name_version")
		path := filepath.Join(dependencyDir, "src", name, "bramble.toml")
		if !fileutil.FileExists(path) {
			return httpx.ErrNotFound(errors.New("can't find package"))
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

func buildJob(job *Job, dependencyDir string, newBuilder types.NewBuilder, downloadGithubRepo func(url string, reference string) (location string, err error)) (builtDerivations []string, err error) {
	loc, err := downloadGithubRepo(job.Package, job.Reference)
	if err != nil {
		return nil, errors.Wrap(err, "error downloading git repo")
	}
	builder, err := newBuilder(loc)
	if err != nil {
		return
	}
	packages := builder.Packages()
	for path, pkg := range packages {
		rel, err := filepath.Rel(loc, path)
		if err != nil {
			panic(loc + " - " + path)
		}
		expectedPackageName := strings.TrimSuffix(job.Package+"/"+strings.Trim(strings.TrimPrefix(rel, "."), "/"), "/")
		if expectedPackageName != pkg.Name {
			return nil, errors.Errorf("package name %q does not match the location the project was fetched from: %q",
				pkg.Name,
				expectedPackageName)
		}
	}
	toRun := []func() error{}
	// Build each package in the repository
	for path, pkg := range packages {
		resp, err := builder.Build(context.Background(), path, nil, types.BuildOptions{Check: true})
		if err != nil {
			return nil, err
		}
		for _, drvFilename := range resp.FinalHashMapping {
			builtDerivations = append(builtDerivations, drvFilename)
		}
		p := pkg // assign to variable to ensure correct value is used
		src := path
		toRun = append(toRun, func() error {
			return addDependencyMetadata(
				dependencyDir,
				p.Name,
				p.Version,
				src,
				resp.Modules)
		})
	}
	// We do this in a separate loop so that we can ensure all builds work
	// before writing packages to the store
	for _, tr := range toRun {
		if err = tr(); err != nil {
			return nil, err
		}
	}
	return builtDerivations, nil
}

func ServerHandler(dependencyDir string, newBuilder types.NewBuilder, dgr types.DownloadGithubRepo) http.Handler {
	return serverHandler(dependencyDir, newBuilder, dgr)
}

func Builder(dependencyDir string, newBuilder types.NewBuilder, dgr types.DownloadGithubRepo) func(*Job) ([]string, error) {
	return func(job *Job) ([]string, error) {
		return buildJob(job, dependencyDir, newBuilder, dgr)
	}
}

func DownloadGithubRepo(url string, reference string) (location string, err error) {
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
