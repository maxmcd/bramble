would be cool to write lots of build scripts in deno/typescript

Could do this by providing a derivation that is pre-built with deno bits, maybe more practical to use Recursive Builds. Could just have a ts lib that either spawned a bramble binary to run builds or talk to the socket. oh man, bramble could call out to a recursive build socket if it found that it was available within an environment. Cool, should get Go working so that we can compile bramble and start running it within itself.

-------------

Kind of a tangent, but seems like recursive builds won't work. The build might use the store and might build derivations, but there's no way to track references to the output of those builds because we can't edit the original derivation to include more dependencies.

Generating a derivation tree as an output of a build seems like an option.

Typescript example then:

I write a derivation like so:

```python
def run_ts(script):
    output = derivation(builder=ts, args=["output-derivations", script])

    return derivation(builder=ts, args["-c", script], env=dict(deps=output))
```

The first step asks for a derivation graph to be generated for all derivations that are needed by the script (annoying, since the script might dynamically ask for different dependencies).

The second step just takes the derivation output and runs the build. The hidden step here is that `output` is actually a tree and all of those dependencies are being built and then combined into a single output drv.

This might be better if we assume that whatever script is being run has a list of dependencies. Then we can use those to spit out the derivation graph in the first step and run it post-build in the second step. This would work well for languages, dependency pulling, etc...

Might bring back the idea of allowing those kinds of derivations to use the network. They could assemble a graph using the network, only output a derivation graph, and then we could store that derivation graph in a lockfile somewhere. No network variability between builds.

--------------


Could have a builder like `derivation_output`. Would take the first argument as the command to run. Would expect a file in the `$out` folder with a json derivation tree to replace that node with in the tree. It's expected that there's one output derivation so that we can pass that on to the next derivation.

So we do have to patch the graph, we have to take the next derivation and replace it's dependencies with the output from this derivation, not the derivation itself.

When we compute this within a project we write the output to a lockfile.

For dependencies we require that the artifacts from a build are stored in the dependency artifact so we can just stick the entire derivation graph there for consumption by other projects, no need to compute that again.


So if there's a typescript lib that talks bramble dependencies and also works with this workflow we just have the tool to spit out the dependency graph?
