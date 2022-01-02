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

	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/netcache"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/v/cmd/go/mvs"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/mod/semver"
)

type Manager struct {
	dependencyDirectory dependencyDirectory
	dependencyClient    *dependencyClient
}

func NewManager(dependencyDir string, packageHost string, cacheClient netcache.Client) *Manager {
	return &Manager{
		dependencyDirectory: dependencyDirectory(dependencyDir),
		dependencyClient: &dependencyClient{
			host:                packageHost,
			cacheClient:         cacheClient,
			dependencyDirectory: dependencyDirectory(dependencyDir),
			client: &http.Client{
				// For tracing
				Transport: otelhttp.NewTransport(http.DefaultTransport),
			},
		},
	}
}

type dependencyDirectory string

func (dd dependencyDirectory) join(v ...string) string {
	return filepath.Join(append([]string{string(dd)}, v...)...)
}

func (dd dependencyDirectory) localPackageVersions(pkg string) ([]string, error) {
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

func (dd dependencyDirectory) localPackageLocation(pkg types.Package) (path string) {
	return dd.join("src", pkg.String())
}

func (dm *Manager) UploadPackage(ctx context.Context, pkg types.Package) (err error) {
	return dm.dependencyClient.uploadPackage(ctx, pkg)
}

func (dm *Manager) PackagePathOrDownload(ctx context.Context, pkg types.Package) (path string, err error) {
	path = dm.dependencyDirectory.localPackageLocation(pkg)
	if fileutil.DirExists(path) {
		return path, nil
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	if err := dm.dependencyClient.getPackageSource(ctx, pkg, path); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	return path, nil
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
	return fileutil.PathExists(dm.dependencyDirectory.localPackageLocation(pkg))
}

func (dm *Manager) localPackageDependencies(pkg types.Package) (vs []types.Package, err error) {
	cfg, err := config.ReadConfig(filepath.Join(dm.dependencyDirectory.localPackageLocation(pkg), "bramble.toml"))
	if err != nil {
		return nil, err
	}
	return configVersions(cfg), nil
}

func (dm *Manager) CalculateConfigBuildlist(ctx context.Context, cfg config.Config) (map[string]config.Dependency, error) {
	versions, err := mvs.BuildList(
		mvsVersionFromPackage(types.Package{Name: cfg.Package.Name, Version: cfg.Package.Version}),
		dm.reqs(cfg),
	)
	if err != nil {
		return nil, err
	}

	buildList := make(map[string]config.Dependency)
	for _, version := range versions {
		v := packageFromMVSVersion(version)
		if v.Name == cfg.Package.Name {
			continue
		}
		// Support path overrides
		buildList[v.Name] = config.Dependency{Version: v.Version}
	}
	return buildList, nil
}

func (dm *Manager) remotePackageDependencies(ctx context.Context, m types.Package) (vs []types.Package, err error) {
	cfg, err := dm.dependencyClient.getPackageConfig(ctx, m)
	if err != nil {
		return nil, err
	}
	return configVersions(cfg.Config), nil
}

func PostJob(ctx context.Context, url, pkg, reference string) (err error) {
	jr := JobRequest{Package: pkg, Reference: reference}
	dc := &dependencyClient{client: &http.Client{}, host: url}
	fmt.Println("Sending build to build server")
	id, err := dc.postJob(context.Background(), jr)
	if err != nil {
		return err
	}
	dur := (time.Millisecond * 1000)
	fmt.Println("Waiting for build result...")
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}
		job, err := dc.getJob(context.Background(), id)
		if err != nil {
			return err
		}
		if job.Error != "" {
			_ = dc.getLogs(context.Background(), id, os.Stdout)
			return errors.Wrap(errors.New(job.ErrWithStack), "got error posting job")
		}
		if !job.End.IsZero() {
			fmt.Printf("Build complete in %s\n", job.End.Sub(job.Start))
			break
		}
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

// Required returns the module versions explicitly required by m itself.
// The caller must not modify the returned list.
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

// Max returns the maximum of v1 and v2 (it returns either v1 or v2).
//
// For all versions v, Max(v, "none") must be v, and for the target passed as
// the first argument to MVS functions, Max(target, v) must be target.
//
// Note that v1 < v2 can be written Max(v1, v2) != v1 and similarly v1 <= v2 can
// be written Max(v1, v2) == v2.
func (r dependencyManagerReqs) Max(v1, v2 string) (o string) {
	if semver.Compare("v0."+v1, "v0."+v2) == -1 {
		return v2
	}
	return v1
}

// Upgrade returns the upgraded version of m, for use during an UpgradeAll
// operation. If m should be kept as is, Upgrade returns m. If m is not yet used
// in the build, then m.Version will be "none". More typically, m.Version will
// be the version required by some other module in the build.
//
// If no module version is available for the given path, Upgrade returns a
// non-nil error.
func (r dependencyManagerReqs) Upgrade(m mvs.Version) (v mvs.Version, err error) {
	panic("unimplemented")
}

// Previous returns the version of m.Path immediately prior to m.Version, or
// "none" if no such version is known.
func (r dependencyManagerReqs) Previous(m mvs.Version) (v mvs.Version, err error) {
	panic("unimplemented")
}

func (dd dependencyDirectory) findPackageFromModuleName(module string) (name string, vs []string, err error) {
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

// FindPackageFromModuleName will search locally for a module, and if it's not
// found it will search remotely for a module. Passing version is optional, but
// if passed it will force a remote search if that version is not found locally
func (dm *Manager) FindPackageFromModuleName(ctx context.Context, module string, version string) (name string, vs []string, err error) {
	// Prefer local
	name, vs, err = dm.dependencyDirectory.findPackageFromModuleName(module)
	if err != nil && err != os.ErrNotExist {
		return "", nil, err
	}
	matchingVersion := func() bool {
		for _, v := range vs {
			if v == version {
				return true
			}
		}
		return false
	}
	versionNotFound := false
	if version != "" {
		versionNotFound = !matchingVersion()
	}

	if err == os.ErrNotExist || versionNotFound {
		name, vs, err = dm.dependencyClient.findPackageFromModuleName(ctx, module)
	}
	if err == os.ErrNotExist {
		return "", nil, errors.Errorf("can't find package for module %q", module)
	}
	return name, vs, err
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
	if fileutil.PathExists(metadataDest) {
		fmt.Println(pkg, version, "already exists locally, skipping writing to store")
		return nil
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
	dependencyDirectory := dependencyDirectory(dependencyDir)

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
			_, _, err := buildJob(c.Request.Context(), job.Package, dependencyDir, newBuilder, downloadGithubRepo)
			jq.End(job.ID, err)
		}()

		return nil
	})
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
		return reptar.Archive(path, c.ResponseWriter)
	})
	router.GET("/package/config/*name_version", func(c httpx.Context) error {
		name := c.Params.ByName("name_version")
		path := filepath.Join(dependencyDir, "src", name)
		if !fileutil.DirExists(path) {
			return httpx.ErrNotFound(errors.New("can't find package"))
		}
		cfg, lockfile, err := config.ReadConfigs(path)
		if err != nil {
			return err
		}
		return json.NewEncoder(c.ResponseWriter).Encode(
			config.ConfigAndLockfile{Config: cfg, Lockfile: lockfile})
	})

	return router
}

// TODO: this is begging to be something other than a heavily overloaded
// function
func buildJob(

	ctx context.Context,
	repo string,
	dependencyDir string,
	newBuilder types.NewBuilder,
	downloadGithubRepo func(url string, reference string) (location string, err error)) (

	builtDerivations []string,
	pkgs []types.Package,
	err error,

) {
	loc, err := downloadGithubRepo(repo, "")
	if err != nil {
		return nil, nil, errors.Wrap(err, "error downloading git repo")
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
		expectedPackageName := strings.TrimSuffix(repo+"/"+strings.Trim(strings.TrimPrefix(rel, "."), "/"), "/")
		if expectedPackageName != pkg.Name {
			return nil, nil, errors.Errorf("package name %q does not match the location the project was fetched from: %q",
				pkg.Name,
				expectedPackageName)
		}
		pkgs = append(pkgs, pkg)
	}
	toRun := []func() error{}
	// Build each package in the repository
	for path, pkg := range packages {
		fmt.Println("Building package", path, pkg)
		resp, err := builder.Build(ctx, path, []string{"./..."}, types.BuildOptions{Check: true})
		if err != nil {
			return nil, nil, err
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
			return nil, nil, err
		}
	}
	return builtDerivations, pkgs, nil
}

func ServerHandler(dependencyDir string, newBuilder types.NewBuilder, dgr types.DownloadGithubRepo) http.Handler {
	return serverHandler(dependencyDir, newBuilder, dgr)
}

func Builder(dependencyDir string, newBuilder types.NewBuilder, dgr types.DownloadGithubRepo) func(context.Context, string) ([]string, []types.Package, error) {
	return func(ctx context.Context, pkg string) ([]string, []types.Package, error) {
		return buildJob(ctx, pkg, dependencyDir, newBuilder, dgr)
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
		// TODO: remove, we should not allow fetches of git repos at references.
		// Otherwise it would be difficult to stage upcoming changes on a public
		// branch.
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
