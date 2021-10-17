package types

// LockfileWriter is used to check if various urls already have a cache entry
// and if it conflicts with a provided cache entry
type LockfileWriter interface {
	AddEntry(string, string) error
	LookupEntry(string) (v string, found bool)
}
