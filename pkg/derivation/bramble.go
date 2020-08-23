// bramble bramble bramble
package derivation

import "github.com/pkg/errors"

var (
	TempDirPrefix         = "bramble-"
	PathPaddingCharacters = "bramble_store_padding"
	PathPaddingLength     = 50

	// BramblePrefixOfRecord is the prefix we use when hashing the build output
	// this allows us to get a consistent hash even if we're building in a
	// different location
	BramblePrefixOfRecord = "/home/bramble/bramble/bramble_store_padding/bramb"
)

var (
	ErrStoreDoesNotExist = errors.New("calculated store path doesn't exist, did the location change?")
)
