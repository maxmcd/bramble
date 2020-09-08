- derivations are left on disk
- derivations can be fetched by hash and name
- outputs can be fetched, this is safe because the derivation contains the system name

If we pack something into a docker container we are limited to just during a derivation into a docker container.

Steps for gc:

 - take all of our derivations that we have tagged as output derivations
 - enumerate their output dependencies, check which output dependencies belong to which input derivations
 - all derivation files and their sources get to stay, delete all outputs we don't depend on
 - delete all recently downloaded files?


----

- keep derivation and sources on the filesystem, that way you can always rebuild the outputs
- remove all derivations that link to nothing
- by keeping the derivations and their sources we will always have a cache hit on starlark parsing, and if an output is needed at a later point we can recalculate it



---

# Description of algorithm

We're interested in a subset of derivation fields:

```go
// Derivation is the basic building block of a Bramble build
type Derivation struct {
	Outputs          map[string]Output
	InputDerivations InputDerivations
	InputSource      InputSource
}
```

- take a derivation we want to keep (is it for runtime or for build input?)
- scan all of the outputs, keep all of the derivations that are used as outputs
- keep all of the input sources
- loop through the inputDerivations and process all of them, keeping them for runtime if they are linked to the output dependencies
