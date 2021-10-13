
- need to be able to specify multiple versions per repo
- will probably want to do that through renaming
- could use semver, but will need to use semver+hash
- could allow one to specify by branch name or hash, will convert to hash
- upgrading will take the latest hash on the default branch
- if you use semver we can look for the latest version tagged with semver

----

Treat every load call like a derivation. Runs git with a network to download the dependencies and the puts them into an output file. The output file is then used as a the location to load the files in the dependency.

Load function will create a derivation, do we add it to the derivation chain for outputs? No, just write it to the dependency file? Add it with a hash, yeah, add the name of the load, any version or hash, and the derivation that's created and the hash of the result. Add all of those to the file? Hash pair definitely goes in the lockfile.

We need git tho, are we re-downlading git every time? So maybe we have a stdlib of derivations and outputs? Those are baked directly into the binary. When we GC we skip them, when we start up or need them we can download them as we want. Do we define these things in starlark? Or build them directly as derivations? (probably derivations?)

So if I want to download a git repo I:
1. hit the load statement
2. somehow identify that the url is a git repo (????)
3. Idenitify the code that is needed to run for git to exist (ah, starlark files in the source repo, bake the resulting derivations, in json, into the binary when building)
4. Run the starlark code and complete the build
5. Get the output location, use that to create a derivation to pull the git repo.
6. That derivation references the full build chain and can be rebuild by the binary.
7. A new version of bramble comes along, it reads the derivation hash, but can't find the matching derivation, what does it do? Hmm, you need to store enough information to fetch a version of bramble and then use that version of bramble to re-download the files you need (recursive build!!!). So we need a bramble verison and enough information to do the full derivation build, and the output hash in order to verify the output. (Why do we actually need to rebuild here? Oh to download the version ourselves, to run that version of the software ourselves.) (oh does that mean running software is always run in a recursive nix build, just using that version of the software needed for the project? I think so, sweeet.)

---

Is there a way to patch dependencies? Can we mess with them if they're derivatioins? Think of something that went through and pruned the version tree?
Maybe just overwrites in the version file for specific deps and their sub-dependencies.

---

Open questions.

- If we can support multiple versions of a dependency in the same repo, how is that specified? If it's specified outside of the bramble.toml file, what's the workflow to get it in the file.
- We should try and support semver early, libraries should define the versions of a software that they support and the depedency resolver should find the smallest subset of libraries that work. This might mean multiple different libc's in the tree, is this a problem? What if we need to support two different versions of software to make it work, do we allow both in the tree? We might need to let users define dependency strategies on subtrees, yikes.
- What is the default version of an import? Less of a problem if we make semver mandatory.
- Gohack equivalent?
- The lockfile is for the project and all of its library dependencies, we ignore the library dependency locks, just take the url locks? Or do we support optionally using the lockfile and dealing with all the related bloat? (What is the solution that puts pressure for the tree to be organized?)
- How do subprojects work, nested bramble.toml files, let's look at rust/cargo.


---

https://github.com/minos-org/minos-static

http://s.minos.io/archive/

---


What if:

1. We use semantic verisioning
2. Every project must state a version
3. You can point at relative dependencies within a project but you must also include a version number and that number must match what is at the relative path
4. There is a public package registry, but it only accepts submissions like this `bramble publish $COMMIT_HASH github.com/maxmcd/bramble/path/to/project`. The registry will then add the version of the package at that commit hash to package registry, there is no authentication, anyone can publish to the registry. If a commit hash is submitted for a version that already has a commit hash then it is rejected. (ignoring the real problem of needing to remove packages).
5. When you call `bramble run github.com/maxmcd/bramble:foo whatever.sh` the package must be published.

```toml
[dependencies]
"github.com/maxmcd/bramble" = "0.1.12"
"github.com/maxmcd/bramble/foo" = {version="0.1.12", path="./foo"}
```


------------------------


We're going with the plan where the dependency server can be prompted to pull from a git/VC repo and then will build and load the derivation into its stores.

From there the dependency server stores references to bramble projects at various versions. When a project tries to load or fetch one of these dependencies the latest version is pulled down and added to the bramble.toml and the lockfile.

So we can start with the server. We'll need something that will accept a git repo location and an optional hash/commit/tag. The server will then pull that repo, download it and build the included derivations. It will check that the project is reproducible and if it is not it will reject it outright.


-------------------

This brought up the idea of function caching. When we pull in dependencies they are frozen, they won't change. So when a function is called and we know that its output is going to be a derivation then we can skip on actually calling and downloading all the dependencies for that function. We can merely crawl the dependency graph, download all the runtime derivations that we'll need to run that derivation and then skip everything else.

If we inject these parentless nodes into the build graph I think it will "just work" since all the local files needed for the build will be there and we don't do exhaustive graph checking before starting a build (we expect the language to do this).

This could also be used to cache in-project builds. If we kept track of file modification times like git we could prune subgraphs from the local project source that we didn't need to rebuild.

---------------

We have a load("github.com/maxmcd/busybox").

1. Looks in the local dependency store for sources
2. If they're not there it checks the remote store
   1. Download if they have it, error if they don't
3. If we have the source we return the path in the store to the execModule input
4. If we have multiple dependencies and those dependencies have dependencies we need to be sure to resolve them all beforehand. We do this by resolving all inputs (Can we confirm there are no unused imports?). When we need to pick versions we pick the latest minor version of each major version. Different major versions get to be imported simultaneously. Projects can pass a version number to an import statement.
