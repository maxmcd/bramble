Currently most code is in the bramble pkg. This package contains starlark logic, store manipulation and build. It would be nice if it was broken down into clearer functional parts.

The store is a core component, I think the store package should be extended to include derivations. Derivations are simply a thing that is stored in the store.

We could have a lang package that implements all starlark functionality.

Then a build package for bramble build?

We need somewhere for the over-the-wire build protocol.


-----------------
```dot
digraph G {
    store -> bramble
    project -> lang -> bramble
    project -> bramble
    store -> lang
    store -> project
    store -> build -> bramble
}
```
store handles storage and builds
builds are independent of packages and projects, they just build derivations that are returned by project and lang

-----------------------
lang has a dependency on store, it would be nice if it was self-contained
currently it needs access to the store in order to:
- write source files to the store
- put files in the star cache

----------------------

- lang doesn't import store.
- source files are archived and a reference to their location on the filesystem is returned
- use a cache library to store starlark cache
- return derivation that is mostly starlark types, an internal representation that can be hashed for caching (skip the sourcefile check)
- higher level lib handles conversion between starlark derivation and real derivation

-------

ok, so derivations complicate this, we kind of want our own version of a
derivation for the lang package, but we also need to inject hashes from other
derivations into the templates. if we imagine we're a library that is implementing
language support for bramble, then it's not too bad if lang is using store and
wrapping the native drv. so we use the native drv, but wrapped. we probably
want to stick the tools that hash up the sources into the project package.
calculating derivation inputs belongs outside of lang.

another thought: we want lib owners to be able to construct json derivations
that are linked to each other and then submit them for builds.

no, lang is external, anyone should be able to use lang, we just happen to use a
specific lang frontend, bramble could support multiple lang frontends, so just
use all the libs you need....?

-----------------

Store is a storage of various derivations and files
Build takes derivation inputs in graph form and the builds them into the store
Project handles project structure
Lang handles starlark files, code execution, modules, dependencies, imports
