// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mvs

import (
	"fmt"
	"strings"
)

// BuildListError decorates an error that occurred gathering requirements
// while constructing a build list. BuildListError prints the chain
// of requirements to the module where the error occurred.
type BuildListError struct {
	Err   error
	stack []buildListErrorElem
}

type buildListErrorElem struct {
	m Version

	// nextReason is the reason this module depends on the next module in the
	// stack. Typically either "requires", or "updating to".
	nextReason string
}

// NewBuildListError returns a new BuildListError wrapping an error that
// occurred at a module found along the given path of requirements and/or
// upgrades, which must be non-empty.
//
// The isUpgrade function reports whether a path step is due to an upgrade.
// A nil isUpgrade function indicates that none of the path steps are due to upgrades.
func NewBuildListError(err error, path []Version, isUpgrade func(from, to Version) bool) *BuildListError {
	stack := make([]buildListErrorElem, 0, len(path))
	for len(path) > 1 {
		reason := "requires"
		if isUpgrade != nil && isUpgrade(path[0], path[1]) {
			reason = "updating to"
		}
		stack = append(stack, buildListErrorElem{
			m:          path[0],
			nextReason: reason,
		})
		path = path[1:]
	}
	stack = append(stack, buildListErrorElem{m: path[0]})

	return &BuildListError{
		Err:   err,
		stack: stack,
	}
}

// Module returns the module where the error occurred. If the module stack
// is empty, this returns a zero value.
func (e *BuildListError) Module() Version {
	if len(e.stack) == 0 {
		return Version{}
	}
	return e.stack[len(e.stack)-1].m
}

func (e *BuildListError) Error() string {
	b := &strings.Builder{}
	stack := e.stack

	// Don't print modules at the beginning of the chain without a
	// version. These always seem to be the main module or a
	// synthetic module ("target@").
	for len(stack) > 0 && stack[0].m.Version == "" {
		stack = stack[1:]
	}

	if len(stack) == 0 {
		b.WriteString(e.Err.Error())
	} else {
		for _, elem := range stack[:len(stack)-1] {
			fmt.Fprintf(b, "%s %s\n\t", elem.m, elem.nextReason)
		}
		// Ensure that the final module path and version are included as part of the
		// error message.
		m := stack[len(stack)-1].m
		fmt.Fprintf(b, "%v %v", m, e.Err)
	}
	return b.String()
}
