Nix handles early cutoff with fixed output derivations. These must be manually kept up to date.

Because the dependency graph in Bramble is (currently) constructed using the hash of input derivations any change to build input will result in a full rebuild.

When thinking about solutions here remember that all build inputs are either filesystem files or `fetch` derivations, so don't get too creative about storing build state.

### Always use the build output as the derivation hash

When currently injecting a derivation into a child derivation we use a format like this: `{{ tmb75glr3iqxaso2gn27ytrmr4ufkv6d-.drv:out }}`. Alternatively we could replace that with the hash of the output.

I think this is what the steps would look like with this approach:

1. Calculate the entire derivation graph. If we already have build outputs computed use them as the named hash.
2. Find the first derivation that needs to be built. Build it, replace all the child derivations derivation hashes with the output hash. Continue building.

What's the cost here? We wouldn't know the derivations we're going to build without building them, this only means that we don't know in advance which things we're going to have to build. This seems fine (and I think is mandatory for this feature).


-----

I think this means we can't trust the derivation name any more, it is subject to change. Every time we build a derivation we must patch all of its references everywhere. This includes derivation inputs and caches and template strings.

So after we've built a derivation, we could freeze the tree and patch all dependent derivations. We know that none of them will be building since they rely on the derivation that's being built. During the patch phase we'll need to hold a lock and then update:
- All references to child derivation names
- All references in derivation outputs
- All text references of derivations in dependent derivtions

When building from scratch we'll need to do this a bit. We'll also need to re-fetch derivations from disk/cache when new names are computed.

----

Shouldn't check to see if a derivation exists on disk until all of it's build inputs have build outputs.
