package bramble

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

var (
	DockerScratchImageName      = "bramble-scratch"
	DockerBramblePathVolumeName = "bramble-path"
)

// TODO: Should be able to download a version of the bramble executable for a specific
// version. The version should be in the derivation and the software should be able to download
// various versions of the software to ensure that it's running starlark within the container correctly
// the bramble binary should be staticly compiled and referenced by hash. We hash the binary and write the
// hash to itself just like we patch our build outputs. The hash should be baked into the binary at compile
// time.
//
// If there isn't a bramble hash or version available we should lookPath at the current environment
// and copy that bramble version into the build. For darwin/windows we might need to reference a
// know location outside of the container or trigger a cross-build or, hmm... something.

type runDockerBuildOptions struct {
	buildDir    string
	outputPaths map[string]string

	stdin io.Reader

	mountBrambleBinary bool
	workingDir         string
	cmd                []string
	env                []string
}

func ensureBrambleVolume(client *docker.Client) (err error) {
	volumes, err := client.ListVolumes(docker.ListVolumesOptions{
		Filters: map[string][]string{"name": {DockerBramblePathVolumeName}},
	})
	if err != nil {
		return err
	}

	if len(volumes) > 0 { // add labels when creating and check them here as a fallback on name collisions
		return nil
	}

	_, err = client.CreateVolume(docker.CreateVolumeOptions{Name: DockerBramblePathVolumeName})
	return err
}

func ensureBrambleScratchImage(client *docker.Client) (err error) {
	images, err := client.ListImages(docker.ListImagesOptions{
		Filter: DockerScratchImageName,
	})
	if err != nil {
		return err
	}
	if len(images) > 0 && images[0].Size == 0 {
		return nil
	}

	// Build scratch image if we don't have it
	buf := bytes.NewBuffer(nil)
	dockerfileContents := "FROM scratch\nCMD nothing"
	tarWriter := tar.NewWriter(buf)
	if err = tarWriter.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Size: int64(len(dockerfileContents)),
	}); err != nil {
		return
	}
	if _, err = tarWriter.Write([]byte(dockerfileContents)); err != nil {
		return
	}
	if err = tarWriter.Close(); err != nil {
		return
	}

	if err := client.BuildImage(docker.BuildImageOptions{
		Name:         DockerScratchImageName,
		InputStream:  buf,
		OutputStream: ioutil.Discard,
	}); err != nil {
		return errors.Wrap(err, "error building bramble-scratch")
	}
	return nil
}

func (b *Bramble) runDockerBuild(ctx context.Context, name string, options runDockerBuildOptions) (err error) {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return err
	}
	if options.buildDir == "" {
		return errors.New("must include a build directory")
	}
	if len(options.outputPaths) == 0 {
		return errors.New("must include output paths")
	}

	binds := []string{
		// mount the entire store path as a ready-only volume
		fmt.Sprintf("%s:%s:ro", b.store.storePath, b.store.storePath),
		fmt.Sprintf("%s:%s", // volume mount the build directory
			options.buildDir,
			options.buildDir,
		),
	}

	if options.mountBrambleBinary {
		// TODO: replace with symlink to store path of the specific bramble
		// version we want
		binds = append(binds, fmt.Sprintf("%s:%s", // bring in a version of bramble
			filepath.Join(b.store.bramblePath, "var/linux-binary"),
			"/bin/bramble",
		))
	}

	for _, outputPath := range options.outputPaths {
		binds = append(binds, fmt.Sprintf("%s:%s", // volume mount all output directories
			outputPath,
			outputPath,
		))
	}

	if err = ensureBrambleScratchImage(client); err != nil {
		return err
	}

	hasStdin := options.stdin != nil

	// TODO: remove when done developing this feature
	_ = client.RemoveContainer(docker.RemoveContainerOptions{ID: name, Force: true})

	cont, err := client.CreateContainer(docker.CreateContainerOptions{
		Name: name,
		Config: &docker.Config{
			User:            fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
			NetworkDisabled: true,

			Image:      "bramble-scratch",
			Cmd:        options.cmd,
			Env:        options.env,
			WorkingDir: options.workingDir,

			AttachStderr: true,
			AttachStdout: true,
			Tty:          false,

			AttachStdin: hasStdin,
			OpenStdin:   hasStdin,
			StdinOnce:   hasStdin,
		},
		HostConfig: &docker.HostConfig{
			Binds: binds,
		},
		Context: ctx,
	})
	if err != nil {
		return errors.Wrap(err, "error creating container")
	}
	_, err = client.AttachToContainerNonBlocking(docker.AttachToContainerOptions{
		Container:    cont.ID,
		Stderr:       true,
		Stdout:       true,
		RawTerminal:  false,
		Stream:       true,
		OutputStream: os.Stdout,
		ErrorStream:  os.Stderr,

		Stdin:       hasStdin,
		InputStream: options.stdin,
	})
	if err != nil {
		return errors.Wrap(err, "error attaching to container")
	}

	if err = client.StartContainer(cont.ID, nil); err != nil {
		return errors.Wrap(err, "error starting container")
	}

	if _, err := client.WaitContainerWithContext(cont.ID, ctx); err != nil {
		return err
	}

	if cont, err = client.InspectContainerWithOptions(docker.InspectContainerOptions{
		ID: cont.ID,
	}); err != nil {
		return err
	}
	if cont.State.Running {
		return errors.New("build container is still running")
	}
	if cont.State.ExitCode != 0 {
		return errors.Errorf("got container exit code %d", cont.State.ExitCode)
	}
	return client.RemoveContainer(docker.RemoveContainerOptions{
		ID: cont.ID,
	})
}

func dockerRunName() string {
	rand.Seed(time.Now().UnixNano())
	return fmt.Sprintf("bramble-run-%d", rand.Int())
}

func (b *Bramble) runDockerRun(ctx context.Context, args []string) (err error) {
	name := dockerRunName()

	client, err := docker.NewClientFromEnv()
	if err != nil {
		return err
	}

	if err = ensureBrambleVolume(client); err != nil {
		return
	}
	if err = ensureBrambleScratchImage(client); err != nil {
		return
	}

	binds := []string{
		// mount the bramble path
		// we use the hosts bramble path here as a convenience so that we don't have
		// to rewrite paths
		fmt.Sprintf("%s:%s", DockerBramblePathVolumeName, b.store.bramblePath),

		// bring in a version of bramble
		fmt.Sprintf("%s:%s",
			filepath.Join(b.store.bramblePath, "var/linux-binary"),
			"/bin/bramble"),

		// Mount the project that we're in
		fmt.Sprintf("%s:%s",
			b.configLocation,
			b.configLocation),

		// pass in the docker sock. this wouldn't support connecting to docker
		// machines, might want to think about supporting that... (TODO)
		"/var/run/docker.sock:/var/run/docker.sock",
	}

	// pass the host environment
	env := []string{}
	// make sure we use the bramble path that we've mounted
	env = append(env, "BRAMBLE_PATH="+b.store.bramblePath)
	// env = append(env, fmt.Sprintf("BRAMBLE_SET_UID=%d", os.Geteuid()))
	// env = append(env, fmt.Sprintf("BRAMBLE_SET_GID=%d", os.Getegid()))
	fmt.Println("creating container with name", name)

	cmd := append([]string{"/bin/bramble"}, args...)

	wd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "getting working directory for docker container")
	}
	fmt.Println("Running createContainer with", cmd, env)
	cont, err := client.CreateContainer(docker.CreateContainerOptions{
		Name: name,
		Config: &docker.Config{
			NetworkDisabled: false,

			// We don't set the user. We start as root and the process
			// calls setuid/setguid
			// User: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),

			Image: "bramble-scratch",
			Cmd:   cmd,
			Env:   env,

			WorkingDir: wd,

			AttachStderr: true,
			AttachStdout: true,
			Tty:          true,

			AttachStdin: true,
			OpenStdin:   true,
			StdinOnce:   true,
		},
		HostConfig: &docker.HostConfig{
			Binds: binds,
		},
		Context: ctx,
	})
	if err != nil {
		return errors.Wrap(err, "error creating container")
	}
	_, err = client.AttachToContainerNonBlocking(docker.AttachToContainerOptions{
		Container:    cont.ID,
		Stdout:       true,
		Stderr:       true,
		Stdin:        true,
		RawTerminal:  true,
		Stream:       true,
		OutputStream: os.Stdout,
		ErrorStream:  os.Stderr,
		InputStream:  os.Stdin,
	})
	if err != nil {
		return errors.Wrap(err, "error attaching to container")
	}

	if err = client.StartContainer(cont.ID, nil); err != nil {
		return errors.Wrap(err, "error starting container")
	}

	if _, err := client.WaitContainerWithContext(cont.ID, ctx); err != nil {
		return err
	}

	if cont, err = client.InspectContainerWithOptions(docker.InspectContainerOptions{
		ID: cont.ID,
	}); err != nil {
		return err
	}
	if cont.State.Running {
		return errors.New("run container is still running")
	}
	if cont.State.ExitCode != 0 {
		return errors.Errorf("got container exit code %d", cont.State.ExitCode)
	}

	return client.RemoveContainer(docker.RemoveContainerOptions{
		ID: cont.ID,
	})
}
