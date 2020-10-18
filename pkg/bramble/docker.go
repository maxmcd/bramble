package bramble

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

var (
	DockerBramblePath      = "/bramble"
	DockerBrambleStorePath = "/bramble/bramble_store_padding/bramble_store_padd"
)

// TODO: should be able to download a version of the bramble executable for a specific
// version. The version should be in the derivation and the software should be able to download
// various versions of the software to ensure that it's running starlark within the container correctly
// the bramble binary should be staticly compiled and referenced by hash. We hash the binary and write the
// hash to itself just like we patch our build outputs. The hash should be baked into the binary at compile
// time.
//
// If there isn't a bramble hash or version available we should lookPath at the current environment
// and copy that bramble version into the build. For darwin/windows we might need to reference a
// know location outside of the container or trigger a cross-build or, hmm... something.

func (b *Bramble) Docker(ctx context.Context,
	buildDir string, outputMap map[string]string,
	drv *Derivation) (err error) {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return err
	}

	dirName, err := calculatePaddedDirectoryName(DockerBramblePath, PathPaddingLength)
	if err != nil {
		return errors.Wrap(err, "error computing bramble store path for a docker build")
	}
	brambleStorePath := filepath.Join(DockerBramblePath, dirName)

	binds := []string{
		// mount the entire store path as a ready-only volume
		fmt.Sprintf("%s:%s:ro", brambleStorePath, b.store.storePath),
		fmt.Sprintf("%s:%s", // volume mount the build directory
			filepath.Join(brambleStorePath, buildDir),
			filepath.Join(b.store.storePath, buildDir),
		),
		// TODO: replace with symlink to store path of the specific bramble
		// version we want
		fmt.Sprintf("%s:%s", // bring in a version of bramble
			"/bin/bramble",
			filepath.Join(b.store.storePath, "/var/linux-binary"),
		),
	}

	for _, outputPath := range outputMap {
		binds = append(binds, fmt.Sprintf("%s:%s", // volume mount all output directories
			filepath.Join(brambleStorePath, outputPath),
			filepath.Join(b.store.storePath, outputPath),
		))
	}

	var env []string
	for k, v := range drv.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// drv.
	cont, err := client.CreateContainer(docker.CreateContainerOptions{
		Name: drv.filename(),
		Config: &docker.Config{
			// TODO match user&group to current user
			NetworkDisabled: true,
			Image:           "scratch",
			Env:             env,
			// Cmd:             options.cmd,
		},
		HostConfig: &docker.HostConfig{
			Binds: binds,
		},
		Context: ctx,
	})
	if err != nil {
		return errors.Wrap(err, "error creating container")
	}

	_ = cont
	return nil
}

func (b *Bramble) DockerContainer(ctx context.Context,
	name string, buildDir string, outputPaths map[string]string,
	stdin io.Reader) (err error) {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return err
	}

	// Uncomment to recalculate:
	// dirName, err := calculatePaddedDirectoryName(DockerBramblePath, PathPaddingLength)
	// if err != nil {
	// 	return errors.Wrap(err, "error computing bramble store path for a docker build")
	// }
	// brambleStorePath := filepath.Join(DockerBramblePath, dirName)
	brambleStorePath := DockerBrambleStorePath

	binds := []string{
		// mount the entire store path as a ready-only volume
		fmt.Sprintf("%s:%s:ro", b.store.storePath, brambleStorePath),
		fmt.Sprintf("%s:%s", // volume mount the build directory
			filepath.Join(b.store.storePath, buildDir),
			filepath.Join(brambleStorePath, buildDir),
		),
		// TODO: replace with symlink to store path of the specific bramble
		// version we want
		fmt.Sprintf("%s:%s", // bring in a version of bramble
			filepath.Join(b.store.bramblePath, "var/linux-binary"),
			"/bin/bramble",
		),
	}

	for _, outputPath := range outputPaths {
		binds = append(binds, fmt.Sprintf("%s:%s", // volume mount all output directories
			filepath.Join(b.store.storePath, outputPath),
			filepath.Join(brambleStorePath, outputPath),
		))
	}

	buf := bytes.NewBuffer(nil)
	dockerfileContents := "FROM scratch\nCMD foo"
	tarWriter := tar.NewWriter(buf)
	_ = tarWriter.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Size: int64(len(dockerfileContents)),
	})
	_, _ = tarWriter.Write([]byte(dockerfileContents))
	_ = tarWriter.Close()

	if err := client.BuildImage(docker.BuildImageOptions{
		Name:         "bramble-scratch",
		InputStream:  buf,
		OutputStream: os.Stderr,
	}); err != nil {
		return errors.Wrap(err, "error building bramble-scratch")
	}

	// TODO: remove when done with deving on this feature
	_ = client.RemoveContainer(docker.RemoveContainerOptions{ID: name, Force: true})

	cont, err := client.CreateContainer(docker.CreateContainerOptions{
		Name: name,
		Config: &docker.Config{
			// TODO match user&group to current user
			User:            fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
			NetworkDisabled: true,
			Image:           "bramble-scratch",
			Cmd:             []string{"/bin/bramble", BrambleFunctionBuildHiddenCommand},

			AttachStderr: true,
			AttachStdout: true,
			AttachStdin:  true,
			Tty:          false,
			OpenStdin:    true,
			StdinOnce:    true,
		},
		HostConfig: &docker.HostConfig{
			Binds: binds,
		},
		Context: ctx,
	})
	if err != nil {
		return errors.Wrap(err, "error creating container")
	}
	success := make(chan struct{})
	// https://gist.github.com/fsouza/43a05241ed9f943d24e5324c0f07471a
	waiter, err := client.AttachToContainerNonBlocking(docker.AttachToContainerOptions{
		Container:    cont.ID,
		Stderr:       true,
		Stdout:       true,
		Stdin:        true,
		RawTerminal:  false,
		Stream:       true,
		InputStream:  stdin,
		OutputStream: os.Stdout,
		ErrorStream:  os.Stderr,
		Success:      success,
	})
	if err != nil {
		return errors.Wrap(err, "error attaching to container")
	}
	err = client.StartContainer(cont.ID, nil)

	if err != nil {
		return errors.Wrap(err, "error starting container")
	}
	<-success
	success <- struct{}{}
	if err := waiter.Wait(); err != nil {
		return err
	}
	if _, err := client.WaitContainer(cont.ID); err != nil {
		return err
	}
	cont, err = client.InspectContainerWithOptions(docker.InspectContainerOptions{
		ID: cont.ID,
	})
	if err != nil {
		return err
	}
	if cont.State.Running {
		return errors.New("container is still running")
	}
	if cont.State.ExitCode != 0 {
		return errors.Errorf("got container exit code %d", cont.State.ExitCode)
	}
	return nil
}
