package types

import "sync"

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
