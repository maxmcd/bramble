package bramble

import (
	"bytes"
	"crypto/sha512"
	"encoding/base32"
	"hash"
	"strings"
)

type Hasher struct {
	hash hash.Hash
}

func NewHasher() *Hasher {
	return &Hasher{
		hash: sha512.New(),
	}
}

func (h *Hasher) Write(b []byte) (n int, err error) {
	return h.hash.Write(b)
}

func (h *Hasher) String() string {
	// We copy nix here
	// https://nixos.org/nixos/nix-pills/nix-store-paths.html
	// Finally the comments tell us to compute the base-32 representation of the
	// first 160 bits (truncation) of a sha256 of the above string:
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(h.hash.Sum(nil)[:20])
	return strings.ToLower(buf.String())
}
