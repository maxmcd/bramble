We'd want to package up git so that we can use it do download git repos.

Could have some kind of stdlib to import from.

That stdlib could be baked into the binary. We output the json derivations from the parse/build and then add a mapping between module imports and functions/derivations (actually not trivial if we want to serialize starlark functions etc...)

Alternatively we could have the stdlib map to files in this repo. In dev mode these files would be pulled from the local repo context (I guess just building things in the github.com/maxmcd/bramble project??) Otherwise it would try and pull the files from a repo (bootstrapping problem can't pull without git, which is external....).


How do I do just the git derivation? I can pull in git, but then I need the starlark source to actually compute the new derivation. Hm. Could just stick the starlark in source like the assert module.

So how does it work?

Could be:

1. Handle it all internally
```
derivation(builder="fetch_git", env=dict(url="github.com/maxmcd/foo"))
```


2. Magic import
```
load("github.com/maxmcd/bramble/git")

def foo:
    git.fetch_git("foo.com:foo.git")
```

3. builtin
How do we pin versions for this one? Oh, do we bootstrap with this and then pull the source repo?
```
fetch_git("foo.com:foo.git")
```

