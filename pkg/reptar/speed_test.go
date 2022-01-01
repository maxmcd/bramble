package reptar

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"golang.org/x/time/rate"
)

type RateLimitReader struct {
	reader  io.Reader
	limiter *rate.Limiter
}

const (
	// Arbitrary
	burstLimit = 1000 * 1000 * 1000

	KiB = 1024
	MiB = 1024 * KiB
)

// SetRateLimit sets rate limit (bytes/sec) to the reader.
func (s *RateLimitReader) SetRateLimit(bytesPerSec float64) {
	s.limiter = rate.NewLimiter(rate.Limit(bytesPerSec), burstLimit)
	s.limiter.AllowN(time.Now(), burstLimit) // spend initial burst
}

func NewLimitReader(r io.Reader) *RateLimitReader {
	return &RateLimitReader{reader: r}
}

func (s *RateLimitReader) Read(p []byte) (int, error) {
	if s.limiter == nil {
		return s.reader.Read(p)
	}
	n, err := s.reader.Read(p)
	if err != nil {
		return n, err
	}
	if err := s.limiter.WaitN(context.Background(), n); err != nil {
		return n, err
	}
	return n, nil
}

func BenchmarkDownload(b *testing.B) {
	var archiveLocation string
	if fileutil.FileExists("./go1.17.2.linux-amd64.tar.gz") {
		archiveLocation, _ = filepath.Abs("./go1.17.2.linux-amd64.tar.gz")
	} else {
		resp, err := http.Get("https://golang.org/dl/go1.17.2.linux-amd64.tar.gz")
		if err != nil {
			b.Fatal(err)
		}
		loc := b.TempDir()
		archiveLocation := filepath.Join(loc, "bootstrap-tools.tar.xz")
		f, err := os.Create(archiveLocation)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
		if err := f.Close(); err != nil {
			b.Fatal(err)
		}

	}
	b.ResetTimer()
	b.Run("baseline", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			f, err := os.Open(archiveLocation)
			if err != nil {
				b.Fatal(err)
			}
			if err := GzipUnarchive(f, b.TempDir()); err != nil {
				b.Fatal(err)
			}
			if err := f.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
	speeds := []int{10 * MiB, 25 * MiB, 100 * MiB, 1000 * MiB}
	for _, speed := range speeds {
		b.Run(fmt.Sprintf("gzip streaming %d MiB/s", speed/MiB), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f, err := os.Open(archiveLocation)
				if err != nil {
					b.Fatal(err)
				}
				rlr := NewLimitReader(f)
				rlr.SetRateLimit(float64(speed))
				reader, err := gzip.NewReader(rlr)
				if err != nil {
					b.Fatal(err)
				}
				if err := Unarchive(reader, b.TempDir()); err != nil {
					b.Fatal(err)
				}
				if err := reader.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("pgzip streaming %d MiB/s", speed/MiB), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f, err := os.Open(archiveLocation)
				if err != nil {
					b.Fatal(err)
				}
				rlr := NewLimitReader(f)
				rlr.SetRateLimit(float64(speed))
				if err := GzipUnarchive(rlr, b.TempDir()); err != nil {
					b.Fatal(err)
				}
				if err := f.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("separate download %d MiB/s", speed/MiB), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f, err := os.Open(archiveLocation)
				if err != nil {
					b.Fatal(err)
				}
				rlr := NewLimitReader(f)
				rlr.SetRateLimit(float64(speed))
				reader, err := gzip.NewReader(rlr)
				if err != nil {
					b.Fatal(err)
				}
				tmpLoc := filepath.Join(b.TempDir(), "tmp")
				tmp, err := os.Create(tmpLoc)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := io.Copy(tmp, reader); err != nil {
					b.Fatal(err)
				}
				_ = tmp.Close()
				tmpToRead, err := os.Open(tmpLoc)
				if err != nil {
					b.Fatal(err)
				}
				if err := Unarchive(tmpToRead, b.TempDir()); err != nil {
					b.Fatal(err)
				}
				if err := reader.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
