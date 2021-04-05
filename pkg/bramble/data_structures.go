package bramble

import (
	"fmt"
	"strings"
	"sync"

	"github.com/maxmcd/dag"
)

type DerivationsMap struct {
	sync.Map
}

func (dm *DerivationsMap) Get(id string) *Derivation {
	d, ok := dm.Load(id)
	if !ok {
		return nil
	}
	return d.(*Derivation)
}

func (dm *DerivationsMap) Has(id string) bool {
	return dm.Get(id) != nil
}
func (dm *DerivationsMap) Set(id string, drv *Derivation) {
	dm.Store(id, drv)
}

// Range calls f sequentially for each key and value present in the map. If f
// returns false, range stops the iteration.
func (dm *DerivationsMap) Range(f func(filename string, drv *Derivation) bool) {
	dm.Map.Range(func(key, value interface{}) bool {
		return f(key.(string), value.(*Derivation))
	})
}

type AcyclicGraph struct {
	dag.AcyclicGraph
}

func NewAcyclicGraph() *AcyclicGraph {
	return &AcyclicGraph{}
}

func (ag AcyclicGraph) PrintDot() {
	graphString := string(ag.Dot(&dag.DotOpts{DrawCycles: true, Verbose: true}))
	fmt.Println(strings.ReplaceAll(graphString, "\"[root] ", "\""))
}

type BiStringMap struct {
	s       sync.RWMutex
	forward map[string]string
	inverse map[string]string
}

// NewBiStringMap returns a an empty, mutable, BiStringMap
func NewBiStringMap() *BiStringMap {
	return &BiStringMap{
		forward: make(map[string]string),
		inverse: make(map[string]string),
	}
}

func (b *BiStringMap) Store(k, v string) {
	b.s.Lock()
	b.forward[k] = v
	b.inverse[v] = k
	b.s.Unlock()
}

func (b *BiStringMap) Load(k string) (v string, exists bool) {
	b.s.RLock()
	v, exists = b.forward[k]
	b.s.RUnlock()
	return
}

func (b *BiStringMap) StoreInverse(k, v string) {
	b.s.Lock()
	b.forward[v] = k
	b.inverse[k] = v
	b.s.Unlock()
}

func (b *BiStringMap) LoadInverse(k string) (v string, exists bool) {
	b.s.RLock()
	v, exists = b.inverse[k]
	b.s.RUnlock()
	return
}
