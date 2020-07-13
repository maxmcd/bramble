package bramble

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Hasher struct {
	hash hash.Hash
}

func NewHasher() *Hasher {
	return &Hasher{
		hash: sha256.New(),
	}
}

func (h *Hasher) Write(b []byte) (n int, err error) {
	return h.hash.Write(b)
}

func (h *Hasher) String() string {
	return bytesToBase32Hash(h.hash.Sum(nil))
}

// bytesToBase32Hash copies nix here
// https://nixos.org/nixos/nix-pills/nix-store-paths.html
// Finally the comments tell us to compute the base-32 representation of the
// first 160 bits (truncation) of a sha256 of the above string:
func bytesToBase32Hash(b []byte) string {
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(b[:20])
	return strings.ToLower(buf.String())
}

func hashDir(location string) (hash string) {
	hasher := NewHasher()
	location = filepath.Clean(location) + "/" // use the extra / to make the paths relative

	// TODO: this is incomplete, ensure you cover the bits that NAR has
	// determined are important
	// https://gist.github.com/jbeda/5c79d2b1434f0018d693

	// TODO: handle common errors like "missing location"
	// likely still want to ignore errors related to missing symlinks, etc...
	// likely with very explicit handling

	// filepath.Walk orders files in lexical order, so this will be deterministic
	_ = filepath.Walk(location, func(path string, info os.FileInfo, _ error) error {
		relativePath := strings.Replace(path, location, "", -1)
		_, _ = hasher.Write([]byte(relativePath))
		f, err := os.Open(path)
		if err != nil {
			// we already know this file exists, likely just a symlink that points nowhere
			fmt.Println(path, err)
			return nil
		}
		_, _ = io.Copy(hasher, f)
		f.Close()
		return nil
	})
	return hasher.String()
}
