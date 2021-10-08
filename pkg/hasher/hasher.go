package hasher

import (
	"bytes"
	"encoding/base32"
	"errors"
	"fmt"
	"hash"
	"strings"

	"github.com/minio/sha256-simd"
)

var ErrHashMismatch = errors.New("two hashes don't match")

// Hasher is used to compute path hash values. Hasher implements io.Writer and
// takes a sha256 hash of the input bytes. The output string is a lowercase
// base32 representation of the first 160 bits of the hash
type Hasher struct {
	hash hash.Hash
}

func New() *Hasher {
	return &Hasher{
		hash: sha256.New(),
	}
}

func (h *Hasher) Write(b []byte) (n int, err error) {
	return h.hash.Write(b)
}

func (h *Hasher) String() string {
	return BytesToBase32Hash(h.hash.Sum(nil))
}
func (h *Hasher) Sha256Hex() string {
	return fmt.Sprintf("%x", h.hash.Sum(nil))
}

func HashString(input string) string {
	h := New()
	_, _ = h.Write([]byte(input))
	return h.String()
}

// BytesToBase32Hash copies nix here
// https://nixos.org/nixos/nix-pills/nix-store-paths.html
// The comments tell us to compute the base32 representation of the
// first 160 bits (truncation) of a sha256 of the above string:
func BytesToBase32Hash(b []byte) string {
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(b[:20])
	return strings.ToLower(buf.String())
}
