package bramble

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/davecgh/go-spew/spew"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

var (
	DockerBramblePath      = "/bramble"
	DockerBrambleStorePath = "/bramble/bramble_store_padding/bramble_store_padd"
	DockerScratchImageName = "bramble-scratch"
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

func genDockerBrambleStorePath() string {
	// Uncomment to recalculate:
	// dirName, err := calculatePaddedDirectoryName(DockerBramblePath, PathPaddingLength)
	// if err != nil {
	// 	return errors.Wrap(err, "error computing bramble store path for a docker build")
	// }
	// brambleStorePath := filepath.Join(DockerBramblePath, dirName)
	return DockerBrambleStorePath
}

type runDockerContainerOptions struct {
	buildDir    string
	outputPaths map[string]string

	stdin io.Reader

	mountBrambleBinary bool
	workingDir         string
	cmd                []string
	env                []string
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

func (b *Bramble) runDockerContainer(ctx context.Context, name string, options runDockerContainerOptions) (err error) {
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
	spew.Dump(options)

	brambleStorePath := genDockerBrambleStorePath()
	binds := []string{
		// mount the entire store path as a ready-only volume
		fmt.Sprintf("%s:%s:ro", b.store.storePath, brambleStorePath),
		fmt.Sprintf("%s:%s", // volume mount the build directory
			filepath.Join(b.store.storePath, options.buildDir),
			filepath.Join(brambleStorePath, options.buildDir),
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
			filepath.Join(b.store.storePath, outputPath),
			filepath.Join(brambleStorePath, outputPath),
		))
	}
	spew.Dump(binds)

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
	return nil
}
