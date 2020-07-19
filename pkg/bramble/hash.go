package bramble

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"hash"
	"os"
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

func IsExecAny(mode os.FileMode) bool {
	return mode&0111 != 0
}
