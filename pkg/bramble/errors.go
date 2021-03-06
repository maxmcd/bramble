package bramble

import (
	"fmt"

	"github.com/pkg/errors"
)

var (
	errQuiet = errors.New("")
)

type errModuleDoesNotExist string

func (err errModuleDoesNotExist) Error() string {
	// TODO: this error is confusing because we can find the module we just
	// can't find the file needed to run/import this specific module path
	return fmt.Sprintf("couldn't find module %q", string(err))
}
