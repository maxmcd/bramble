// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mvs implements Minimal Version Selection.
// See https://research.swtch.com/vgo-mvs.
package mvs

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/maxmcd/bramble/v/cmd/go/par"
)

type Version struct {
	Name    string
	Version string
}

// A Reqs is the requirement graph on which Minimal Version Selection (MVS) operates.
//
// The version strings are opaque except for the special version "none"
// (see the documentation for Version). In particular, MVS does not
// assume that the version strings are semantic versions; instead, the Max method
// gives access to the comparison operation.
//
// It must be safe to call methods on a Reqs from multiple goroutines simultaneously.
// Because a Reqs may read the underlying graph from the network on demand,
// the MVS algorithms parallelize the traversal to overlap network delays.
type Reqs interface {
	// Required returns the module versions explicitly required by m itself.
	// The caller must not modify the returned list.
	Required(m Version) ([]Version, error)

	// Max returns the maximum of v1 and v2 (it returns either v1 or v2).
	//
	// For all versions v, Max(v, "none") must be v,
	// and for the target passed as the first argument to MVS functions,
	// Max(target, v) must be target.
	//
	// Note that v1 < v2 can be written Max(v1, v2) != v1
	// and similarly v1 <= v2 can be written Max(v1, v2) == v2.
	Max(v1, v2 string) string

	// Upgrade returns the upgraded version of m,
	// for use during an UpgradeAll operation.
	// If m should be kept as is, Upgrade returns m.
	// If m is not yet used in the build, then m.Version will be "none".
	// More typically, m.Version will be the version required
	// by some other module in the build.
	//
	// If no module version is available for the given path,
	// Upgrade returns a non-nil error.
	// TODO(rsc): Upgrade must be able to return errors,
	// but should "no latest version" just return m instead?
	Upgrade(m Version) (Version, error)

	// Previous returns the version of m.Path immediately prior to m.Version,
	// or "none" if no such version is known.
	Previous(m Version) (Version, error)
}

// BuildList returns the build list for the target module.
//
// target is the root vertex of a module requirement graph. For cmd/go, this is
// typically the main module, but note that this algorithm is not intended to
// be Go-specific: module paths and versions are treated as opaque values.
//
// reqs describes the module requirement graph and provides an opaque method
// for comparing versions.
//
// BuildList traverses the graph and returns a list containing the highest
// version for each visited module. The first element of the returned list is
// target itself; reqs.Max requires target.Version to compare higher than all
// other versions, so no other version can be selected. The remaining elements
// of the list are sorted by path.
//
// See https://research.swtch.com/vgo-mvs for details.
func BuildList(target Version, reqs Reqs) ([]Version, error) {
	return buildList(target, reqs, nil)
}

func buildList(target Version, reqs Reqs, upgrade func(Version) (Version, error)) ([]Version, error) {
	// Explore work graph in parallel in case reqs.Required
	// does high-latency network operations.
	type modGraphNode struct {
		m        Version
		required []Version
		upgrade  Version
		err      error
	}
	var (
		mu       sync.Mutex
		modGraph = map[Version]*modGraphNode{}
		min      = map[string]string{} // maps module path to minimum required version
		haveErr  int32
	)
	setErr := func(n *modGraphNode, err error) {
		n.err = err
		atomic.StoreInt32(&haveErr, 1)
	}

	var work par.Work
	work.Add(target)
	work.Do(10, func(item interface{}) {
		m := item.(Version)

		node := &modGraphNode{m: m}
		mu.Lock()
		modGraph[m] = node
		if m.Version != "none" {
			if v, ok := min[m.Name]; !ok || reqs.Max(v, m.Version) != v {
				min[m.Name] = m.Version
			}
		}
		mu.Unlock()

		if m.Version != "none" {
			required, err := reqs.Required(m)
			if err != nil {
				setErr(node, err)
				return
			}
			node.required = required
			for _, r := range node.required {
				work.Add(r)
			}
		}

		if upgrade != nil {
			u, err := upgrade(m)
			if err != nil {
				setErr(node, err)
				return
			}
			if u != m {
				node.upgrade = u
				work.Add(u)
			}
		}
	})

	// If there was an error, find the shortest path from the target to the
	// node where the error occurred so we can report a useful error message.
	if haveErr != 0 {
		// neededBy[a] = b means a was added to the module graph by b.
		neededBy := make(map[*modGraphNode]*modGraphNode)
		q := make([]*modGraphNode, 0, len(modGraph))
		q = append(q, modGraph[target])
		for len(q) > 0 {
			node := q[0]
			q = q[1:]

			if node.err != nil {
				pathUpgrade := map[Version]Version{}

				// Construct the error path reversed (from the error to the main module),
				// then reverse it to obtain the usual order (from the main module to
				// the error).
				errPath := []Version{node.m}
				for n, prev := neededBy[node], node; n != nil; n, prev = neededBy[n], n {
					if n.upgrade == prev.m {
						pathUpgrade[n.m] = prev.m
					}
					errPath = append(errPath, n.m)
				}
				i, j := 0, len(errPath)-1
				for i < j {
					errPath[i], errPath[j] = errPath[j], errPath[i]
					i++
					j--
				}

				isUpgrade := func(from, to Version) bool {
					return pathUpgrade[from] == to
				}

				return nil, NewBuildListError(node.err, errPath, isUpgrade)
			}

			neighbors := node.required
			if node.upgrade.Name != "" {
				neighbors = append(neighbors, node.upgrade)
			}
			for _, neighbor := range neighbors {
				nn := modGraph[neighbor]
				if neededBy[nn] != nil {
					continue
				}
				neededBy[nn] = node
				q = append(q, nn)
			}
		}
	}

	// The final list is the minimum version of each module found in the graph.

	if v := min[target.Name]; v != target.Version {
		// target.Version will be "" for modload, the main client of MVS.
		// "" denotes the main module, which has no version. However, MVS treats
		// version strings as opaque, so "" is not a special value here.
		// See golang.org/issue/31491, golang.org/issue/29773.
		panic(fmt.Sprintf("mistake: chose version %q instead of target %+v", v, target)) // TODO: Don't panic.
	}

	list := []Version{target}
	for path, vers := range min {
		if path != target.Name {
			list = append(list, Version{Name: path, Version: vers})
		}

		n := modGraph[Version{Name: path, Version: vers}]
		required := n.required
		for _, r := range required {
			if r.Version == "none" {
				continue
			}
			v := min[r.Name]
			if r.Name != target.Name && reqs.Max(v, r.Version) != v {
				panic(fmt.Sprintf("mistake: version %q does not satisfy requirement %+v", v, r)) // TODO: Don't panic.
			}
		}
	}

	tail := list[1:]
	sort.Slice(tail, func(i, j int) bool {
		return tail[i].Name < tail[j].Name
	})
	return list, nil
}

// Req returns the minimal requirement list for the target module,
// with the constraint that all module paths listed in base must
// appear in the returned list.
func Req(target Version, base []string, reqs Reqs) ([]Version, error) {
	list, err := BuildList(target, reqs)
	if err != nil {
		return nil, err
	}

	// Note: Not running in parallel because we assume
	// that list came from a previous operation that paged
	// in all the requirements, so there's no I/O to overlap now.

	// Compute postorder, cache requirements.
	var postorder []Version
	reqCache := map[Version][]Version{}
	reqCache[target] = nil
	var walk func(Version) error
	walk = func(m Version) error {
		_, ok := reqCache[m]
		if ok {
			return nil
		}
		required, err := reqs.Required(m)
		if err != nil {
			return err
		}
		reqCache[m] = required
		for _, m1 := range required {
			if err := walk(m1); err != nil {
				return err
			}
		}
		postorder = append(postorder, m)
		return nil
	}
	for _, m := range list {
		if err := walk(m); err != nil {
			return nil, err
		}
	}

	// Walk modules in reverse post-order, only adding those not implied already.
	have := map[Version]bool{}
	walk = func(m Version) error {
		if have[m] {
			return nil
		}
		have[m] = true
		for _, m1 := range reqCache[m] {
			walk(m1)
		}
		return nil
	}
	max := map[string]string{}
	for _, m := range list {
		if v, ok := max[m.Name]; ok {
			max[m.Name] = reqs.Max(m.Version, v)
		} else {
			max[m.Name] = m.Version
		}
	}
	// First walk the base modules that must be listed.
	var min []Version
	for _, path := range base {
		m := Version{Name: path, Version: max[path]}
		min = append(min, m)
		walk(m)
	}
	// Now the reverse postorder to bring in anything else.
	for i := len(postorder) - 1; i >= 0; i-- {
		m := postorder[i]
		if max[m.Name] != m.Version {
			// Older version.
			continue
		}
		if !have[m] {
			min = append(min, m)
			walk(m)
		}
	}
	sort.Slice(min, func(i, j int) bool {
		return min[i].Name < min[j].Name
	})
	return min, nil
}

// UpgradeAll returns a build list for the target module
// in which every module is upgraded to its latest version.
func UpgradeAll(target Version, reqs Reqs) ([]Version, error) {
	return buildList(target, reqs, func(m Version) (Version, error) {
		if m.Name == target.Name {
			return target, nil
		}

		return reqs.Upgrade(m)
	})
}

// Upgrade returns a build list for the target module
// in which the given additional modules are upgraded.
func Upgrade(target Version, reqs Reqs, upgrade ...Version) ([]Version, error) {
	list, err := reqs.Required(target)
	if err != nil {
		return nil, err
	}

	pathInList := make(map[string]bool, len(list))
	for _, m := range list {
		pathInList[m.Name] = true
	}
	list = append([]Version(nil), list...)

	upgradeTo := make(map[string]string, len(upgrade))
	for _, u := range upgrade {
		if !pathInList[u.Name] {
			list = append(list, Version{Name: u.Name, Version: "none"})
		}
		if prev, dup := upgradeTo[u.Name]; dup {
			upgradeTo[u.Name] = reqs.Max(prev, u.Version)
		} else {
			upgradeTo[u.Name] = u.Version
		}
	}

	return buildList(target, &override{target, list, reqs}, func(m Version) (Version, error) {
		if v, ok := upgradeTo[m.Name]; ok {
			return Version{Name: m.Name, Version: v}, nil
		}
		return m, nil
	})
}

// Downgrade returns a build list for the target module
// in which the given additional modules are downgraded,
// potentially overriding the requirements of the target.
//
// The versions to be downgraded may be unreachable from reqs.Latest and
// reqs.Previous, but the methods of reqs must otherwise handle such versions
// correctly.
func Downgrade(target Version, reqs Reqs, downgrade ...Version) ([]Version, error) {
	list, err := reqs.Required(target)
	if err != nil {
		return nil, err
	}
	max := make(map[string]string)
	for _, r := range list {
		max[r.Name] = r.Version
	}
	for _, d := range downgrade {
		if v, ok := max[d.Name]; !ok || reqs.Max(v, d.Version) != d.Version {
			max[d.Name] = d.Version
		}
	}

	var (
		added    = make(map[Version]bool)
		rdeps    = make(map[Version][]Version)
		excluded = make(map[Version]bool)
	)
	var exclude func(Version)
	exclude = func(m Version) {
		if excluded[m] {
			return
		}
		excluded[m] = true
		for _, p := range rdeps[m] {
			exclude(p)
		}
	}
	var add func(Version)
	add = func(m Version) {
		if added[m] {
			return
		}
		added[m] = true
		if v, ok := max[m.Name]; ok && reqs.Max(m.Version, v) != v {
			exclude(m)
			return
		}
		list, err := reqs.Required(m)
		if err != nil {
			// If we can't load the requirements, we couldn't load the go.mod file.
			// There are a number of reasons this can happen, but this usually
			// means an older version of the module had a missing or invalid
			// go.mod file. For example, if example.com/mod released v2.0.0 before
			// migrating to modules (v2.0.0+incompatible), then added a valid go.mod
			// in v2.0.1, downgrading from v2.0.1 would cause this error.
			//
			// TODO(golang.org/issue/31730, golang.org/issue/30134): if the error
			// is transient (we couldn't download go.mod), return the error from
			// Downgrade. Currently, we can't tell what kind of error it is.
			exclude(m)
		}
		for _, r := range list {
			add(r)
			if excluded[r] {
				exclude(m)
				return
			}
			rdeps[r] = append(rdeps[r], m)
		}
	}

	var out []Version
	out = append(out, target)
List:
	for _, r := range list {
		add(r)
		for excluded[r] {
			p, err := reqs.Previous(r)
			if err != nil {
				// This is likely a transient error reaching the repository,
				// rather than a permanent error with the retrieved version.
				//
				// TODO(golang.org/issue/31730, golang.org/issue/30134):
				// decode what to do based on the actual error.
				return nil, err
			}
			// If the target version is a pseudo-version, it may not be
			// included when iterating over prior versions using reqs.Previous.
			// Insert it into the right place in the iteration.
			// If v is excluded, p should be returned again by reqs.Previous on the next iteration.
			if v := max[r.Name]; reqs.Max(v, r.Version) != v && reqs.Max(p.Version, v) != p.Version {
				p.Version = v
			}
			if p.Version == "none" {
				continue List
			}
			add(p)
			r = p
		}
		out = append(out, r)
	}

	return out, nil
}

type override struct {
	target Version
	list   []Version
	Reqs
}

func (r *override) Required(m Version) ([]Version, error) {
	if m == r.target {
		return r.list, nil
	}
	return r.Reqs.Required(m)
}
