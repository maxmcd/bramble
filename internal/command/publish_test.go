package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmcd/bramble/internal/store"
	"github.com/maxmcd/bramble/pkg/s3test"
)

var (
	lock string = `[URLHashes]
"basic_fetch_url https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz" = "uw5ichj6dhcccmcts6p7jq6etzlh5baf"
"basic_fetch_url https://brmbl.s3.amazonaws.com/url_fetcher.tar.gz" = "p2vbvabkdqckjlm43rf7bfccdseizych"
"fetch_url https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz" = "uw5ichj6dhcccmcts6p7jq6etzlh5baf"`
	toml string = `[package]
name = "github.com/maxmcd/busybox"
version = "0.0.2"`
	main string = `def busybox():
  b = derivation(
    name="busybox-x86_64.tar.gz",
    builder="fetch_url",
    env={"url": "https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz"})

  script = """
  set -ex
  # cachebust
  $busybox_download/busybox-x86_64 mkdir $out/bin
  $busybox_download/busybox-x86_64 cp $busybox_download/busybox-x86_64 $out/bin/busybox
  cd $out/bin
  for command in $(./busybox --list); do
    ./busybox ln -s busybox $command
  done
  """

  return derivation(
    name="busybox",
    builder=b.out + "/busybox-x86_64",
    args=["sh", "-c", script],
    env={"busybox_download": b, "PATH": b.out},
  )`
)

func TestPublish(t *testing.T) {
	server := s3test.StartServer(t, ":0")
	cc := store.NewS3CacheClient(" ", " ", server.Hostname())
	cc.PathStyle = true
	cc.Scheme = "http"

	if err := publish(context.Background(), publishOptions{
		pkg:    "github.com/maxmcd/busybox",
		upload: true,
		local:  true,
	}, func(url, reference string) (location string, err error) {
		location = t.TempDir()
		_ = os.WriteFile(filepath.Join(location, "bramble.toml"), []byte(toml), 0755)
		_ = os.WriteFile(filepath.Join(location, "bramble.lock"), []byte(lock), 0755)
		_ = os.WriteFile(filepath.Join(location, "default.bramble"), []byte(main), 0755)
		return location, nil
	}, cc); err != nil {
		t.Fatal(err)
	}
}
