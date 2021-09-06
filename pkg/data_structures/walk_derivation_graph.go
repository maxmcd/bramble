package ds

import (
	"fmt"
	"sync"

	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
)

// drvReplaceableMap provides a map of DrvReplacables that is guarded by a mutex.
// You can also retrieve derivations from map to work on using
// Lock(hash string). The lock must be released with Unlock(hash string) in
// order for work to be done on that derivation elsewhere.
type drvReplaceableMap struct {
	drvs  map[string]DrvReplacable
	lock  sync.Mutex
	locks map[string]*sync.Mutex
}

func newDrvReplaceableMap() *drvReplaceableMap {
	return &drvReplaceableMap{
		drvs:  map[string]DrvReplacable{},
		locks: map[string]*sync.Mutex{},
	}
}

func (drm *drvReplaceableMap) add(drv DrvReplacable) {
	drm.lock.Lock()
	drm.drvs[drv.Hash()] = drv
	drm.lock.Unlock()
}

func (drm *drvReplaceableMap) lockDrv(hash string) (drv DrvReplacable, found bool) {
	drm.lock.Lock()

	drv, found = drm.drvs[hash]
	if !found {
		drm.lock.Unlock()
		return drv, found
	}
	lock := drm.locks[hash]
	if lock == nil {
		lock = &sync.Mutex{}
		drm.locks[hash] = lock
	}
	// We must onlock before calling lock on the found mutex in case it blocks
	drm.lock.Unlock()

	lock.Lock()
	return drv, found
}

func (drm *drvReplaceableMap) unlockDrv(hash string) {
	drm.lock.Lock()
	lock := drm.locks[hash]
	drm.lock.Unlock()
	if lock == nil {
		// noop
		return
	}
	lock.Unlock()
}

type derivationGraph struct {
	ag dag.AcyclicGraph

	drvs drvReplaceableMap
}

type DerivationOutput interface {
	Hash() string
	Output() string
}

type DrvReplacable interface {
	ReplaceHash(old, new string)
	Hash() string
}

func newDerivationGraph() *derivationGraph {
	return &derivationGraph{
		drvs: *newDrvReplaceableMap(),
	}
}

func (dg *derivationGraph) addDrv(drv DrvReplacable) {
	dg.drvs.add(drv)
}

func (dg *derivationGraph) connect(dge DerivationGraphEdge) {
	dg.ag.Add(dge.source)
	dg.ag.Add(dge.target)
	dg.ag.Connect(dag.BasicEdge(dge.source, dge.target))
}

func NewDerivationGraphEdge(dependent, dependency DerivationOutput) DerivationGraphEdge {
	return DerivationGraphEdge{
		target: dependency,
		source: dependent,
	}
}

type DerivationGraphEdge struct {
	source DerivationOutput
	target DerivationOutput
}

type WalkDerivationGraphOptions struct {
	Edges       []DerivationGraphEdge
	Derivations []DrvReplacable
	// MaxParallel limits the parallelism of the graph walk. Will be unlimited
	// if MaxParallel is 0
	MaxParallel int
}

// WalkDerivationGraphFunc is callback func passed to WalkDerivationGraph
type WalkDerivationGraphFunc func(do DerivationOutput, drv DrvReplacable) (newHash string, err error)

// WalkDerivationGraph will walk a DerivationGraph. This function is used to do
// work on every node in the graph and then replace a value in any edges to that
// node. Specifically, we use this to construct a graph of DerivationOutputs
// and then replace the contents of each node and dependent node. This is used
// to build derivations (the build will change the output hash in child
// derivations) or to convert starlark derivations into store derivations (the
// build will change derivation hash as sources are added).
//
// A lock is placed on each derivation when it is passed to the callback to
// ensure we only build a given derivation once.
func WalkDerivationGraph(options WalkDerivationGraphOptions, fn WalkDerivationGraphFunc) error {
	// TODO: error handling!

	if fn == nil {
		return errors.New("walk function can't be nil")
	}

	dg := newDerivationGraph()
	for _, drv := range options.Derivations {
		dg.addDrv(drv)
	}
	for _, edge := range options.Edges {
		dg.connect(edge)
	}

	if err := dg.ag.Validate(); err != nil {
		return err
	}
	cache := map[string]string{}
	cacheLock := sync.RWMutex{}

	semaphore := make(chan struct{}, options.MaxParallel)
	errs := dg.ag.Walk(func(v dag.Vertex) error {
		if v == FakeDAGRoot {
			return nil
		}
		// Limit parallism
		if options.MaxParallel != 0 {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
		}

		do := v.(DerivationOutput)
		oldHash := do.Hash()
		// Find the derivation related to this DerivationOutput
		drv, found := dg.drvs.lockDrv(oldHash)
		if !found {
			return errors.Errorf("derivation not found in DerivationGraph with hash %q", oldHash)
		}
		var newHash string
		var err error
		cacheLock.RLock()
		newHash, ok := cache[oldHash]
		cacheLock.RUnlock()
		if !ok { // cached response not found, make the newHash
			newHash, err = fn(do, drv) // do work
			cacheLock.Lock()
			cache[oldHash] = newHash
			cacheLock.Unlock()
			if err != nil {
				return err
			}
		}
		dg.drvs.unlockDrv(oldHash) // free the resource
		// Now find all immediate dependents of this output and patch them to
		// contain the new hash value.
		for _, edge := range dg.ag.EdgesTo(v) {
			if edge.Source() == FakeDAGRoot {
				continue
			}
			do := edge.Source().(DerivationOutput)
			edgeDOHash := do.Hash()
			drv, found := dg.drvs.lockDrv(edgeDOHash)
			if !found {
				return errors.Errorf("derivation not found in DerivationGraph with hash %q", oldHash)
			}
			drv.ReplaceHash(oldHash, newHash)
			dg.drvs.unlockDrv(edgeDOHash)
		}
		return nil
	})
	if len(errs) != 0 {
		return errors.New(fmt.Sprint(errs))
	}
	return nil
}
