package starutil

type DerivationChecker interface {
	// AfterDerivation is called when it is assumed that derivation calls are
	// complete. Derivations will usually build at this point
	AfterDerivation()
	// CallDerivation is called within every derivation call. If this is called
	// after AfterDerivation it will error
	CalledDerivation() (int, map[string]string, error)
}
