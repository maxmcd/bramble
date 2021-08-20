
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
