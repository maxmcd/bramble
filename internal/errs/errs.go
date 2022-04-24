package errs

import "fmt"

type ErrModuleNotFoundInProject struct {
	Module string
}

func (e ErrModuleNotFoundInProject) Error() string {
	return fmt.Sprintf("%q is not a dependency of this project, do you need to add it?", e.Module)
}
func (e ErrModuleNotFoundInProject) Is(err error) bool {
	_, ok := err.(ErrModuleNotFoundInProject)
	return ok
}
