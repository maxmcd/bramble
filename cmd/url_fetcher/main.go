package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/certifi/gocertifi"
	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
)

var (
	// certPool will never error, see gocertifi docs
	certPool  = func() *x509.CertPool { certPool, _ := gocertifi.CACerts(); return certPool }()
	transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{RootCAs: certPool},
	}
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("%+v\n", err)
		panic("")
	}
}

func run() error {
	url := os.Getenv("url")
	client := http.Client{Transport: transport}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "error requesting url %q", url)
	}
	defer resp.Body.Close()
	wd, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}
	dir, err := os.MkdirTemp(wd, "")
	if err != nil {
		return errors.WithStack(err)
	}
	f, err := os.Create(filepath.Join(dir, filepath.Base(url)))
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		return errors.WithStack(err)
	}
	if err := f.Close(); err != nil {
		return errors.WithStack(err)
	}
	if err := archiver.Unarchive(f.Name(), os.Getenv("out")); err != nil {
		if !strings.Contains(err.Error(), "format unrecognized by filename") {
			return errors.Wrap(err, "error unpacking url archive")
		}
		// Regrettable, but we can't rename/mv files across mounted devices in the sandbox
		return copyFile(f.Name(), filepath.Join(os.Getenv("out"), filepath.Base(url)))
	}
	return nil
}

func copyFile(srcFile, dstFile string) error {
	in, err := os.Open(srcFile)
	if err != nil {
		return errors.WithStack(err)
	}
	defer in.Close()

	fi, err := in.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	out, err := os.OpenFile(dstFile, os.O_CREATE|os.O_RDWR, fi.Mode())
	if err != nil {
		return errors.WithStack(err)
	}

	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
