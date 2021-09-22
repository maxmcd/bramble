DAO

Derivations as output is allowing a derivation to return serialized derivations that are then added back to the build graph. (looks like we've got the lib support at least: https://pkg.go.dev/github.com/maxmcd/dag#Walker.Update).

The output of this derivation is similar to `brambleproject.ExecModuleOutput`:
```go
type ExecModuleOutput struct {
	Output         project.Derivation // just one, regular struct has an array, but we'll need to enforce a single output
	AllDerivations []project.Derivation
}
```

We want all derivations ordered in a graph and then we want a specific output derivation that can be used as the higher-level derivations output.

Simple example:

```python
def dao():
    a = derivation("first", "sh", args=["-c", """
    set -e
    echo "{..some json describing lots of derivations..}" > $out/derivations.json
    """])
    # this now references the results of building the build graph
    print(a.out)
```

When we get a derivation like this we'd have to build it and then add the nodes to build back in the graph. The process that builds the meta derivation would have to pause and wait for the rest of the graph to build before finishing its build. We'd probably want to push a struct back onto the semaphore to ensure that we don't have > NUM_THREADS waiting and locking. Once all of those are built we'd need to somehow wake the paused job.

Could have a pubsub listener that sends out messages when builds are finished. We could subscribe to all build outputs, then push the nodes on the graph, then count and wait for every build to be complete, then destroy the listener and finish our build.

The final derivation then gets outputted as the final result of that derivation. Seems casually that this might be ok, the output would look like it was all originally built as separate derivations generated from starlark.

What about caching? We have two outputs the derivation definitions passed at first and then the final output along with all other other derivations that have been created. Original thought was that we stick the first part into the `.lock` file. This would stick a big json blob in the lockfile. It would mean though that we always had regular derivations to build if we just assume what's in that file is gold. This doesn't work if we have lots of sources! Hmm. So:

1. Don't allow any derivations with sources.
   1. Inline scripts and fetch derivations only.
   2. Should be good if you can pull from git repos and such, can pull any files that are needed and build.
2. Store the sources somewhere.
   1. Hard, already store things in places tough to track arbitrary amounts of source files.

Ok no source files in generated derivations...

Does this actually help with programming languages and their dependency managers?

Yes, for two reasons:
1. If you want to make it hard you can go no-network by using this pattern to use regular fetch builders and then generate different derivations to build based off of the response from those fetch builders.
2. Alteratively, if this is where we allow the network then a regular program could just request various URLs, compute dependency locations and then generate the needed derivations to get the final files together.

Storing the json output is still tough. We can store it, but then how do we reference it? Could use a hash of the derivation, but that only works if we compute the same derivation hash. Could keep computing things like that by calling all functions in all files. That won't work if we ever allowed users to pass parameters to bramble. For now we just do that? Would be quick, just call all functions and compute the initial hashes, no source grabbing or nothin.

Plan to try this out with bramble to compile bramble:

1. Get Go working
2. Compile a higher version of Go
3. Write a program to use go.mod and go.sum to compute the urls that we need to pull for each dependency.
4. Use that program to generate the derivations we need to build for this project.
5. Build a GOPATH on the fly with all those outputs and then compile directly? Not sure if we can use GOPATH or if we need to populate the go module directories.


-----

## How the graph patch works


