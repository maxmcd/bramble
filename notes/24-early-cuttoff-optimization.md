Nix handles early cutoff with fixed output derivations. These must be manually kept up to date.

Because the dependency graph in Bramble is (currently) constructed using the hash of input derivations any change to build input will result in a full rebuild.

I think it's important when thinking about a solution here to remember that all build inputs are either filesystem files or `fetch` derivations, so don't get too creative about storing build state.

### Always use the build output as the derivation hash

When currently injecting a derivation into a child derivation we use a format like this: `{{ tmb75glr3iqxaso2gn27ytrmr4ufkv6d-.drv:out }}`. Alternatively we could replace that with the hash of the output.

I think this is what the steps would look like with this approach:

1. Calculate the entire derivation graph. If we already have build outputs computed use them as the named hash.
2. Find the first derivation that needs to be built. Build it, replace all the child derivations derivation hashes with the output hash. Continue building.

What's the cost here? We wouldn't know the derivations we're going to build without building them, this only means that we don't know in advance which things we're going to have to build. This seems fine (and I think is mandatory for this feature).
