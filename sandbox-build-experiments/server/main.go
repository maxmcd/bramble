package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"

	"github.com/maxmcd/bramble/pkg/bramblepb"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

type BuildServer struct {
}

var _ bramblepb.SingleBuilderServer = new(BuildServer)

func (bs *BuildServer) FetchBuild(server bramblepb.SingleBuilder_FetchBuildServer) (err error) {
	for {
		// TODO: take build from channel
		fmt.Println("sending")
		if err := server.Send(&bramblepb.BuildInput{
			FunctionBuild: &bramblepb.FunctionBuild{
				BuildDirectory: "hello",
			},
		}); err != nil {
			return err
		}
		for {
			status, err := server.Recv()
			if err != nil {
				return err
			}
			if status.Complete {
				break
			}
		}
	}
}

func run() (err error) {
	socketDir, err := ioutil.TempDir("", "bramble-")
	if err != nil {
		return errors.Wrap(err, "tempdir")
	}
	defer os.RemoveAll(socketDir)
	socket := socketDir + "/bramble.sock"

	fmt.Println(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	bramblepb.RegisterSingleBuilderServer(grpcServer, new(BuildServer))
	go func() {
		log.Fatal(grpcServer.Serve(listener))
	}()
	cmd := exec.Command("./sbx", socket)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}
