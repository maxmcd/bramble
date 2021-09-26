
We should remove mholt/achiver and any other built-in software that we should more reasonably. Now that build and project are separatated we can just define the derivations we would need in the input format that the build expects. We can feed in derivation inputs and then write our source into the store (if needed). These derivations can download programs like git and tar at fixed versions. These dependencies will show up in the bramble.toml file so that a project can be guaranteed to see the same versions of git/tar/whatever to rebuild the software that they've fetched


We can even have the derivation for older versions of bramble. When a project needs an old version of bramble we just store all old versions of bramble to download and run (this is trivial now that we don't need root). Project specifies a specific version of bramble we download and run that instead (another reason to keep the binary small). Ah, could even keep libcontainer just in a sandbox utility that we download to run builds. Oof, interesting idea.


Write a tool that takes starlark derivations and turns them into go code. Generate the resulting derivations and then output go code with the structs.


----------------------------

So fetch_url should be a derivation because we need to track that it is a derivation. We need the reproducability and the external dependency. Better than keeping the built-in fetch_url a fixed thing.

So when a person passes a "fetch_url" builder what happens? Currently we run git by running a separate build that's called by the fetch_url builder. We might want to use the patch graph functionality instead? But then how do we tie it into the regular dependency system?

We could have the url func match to a url that we patch. So derivation is actually:

```
def derivation(name, builder):
    if builder == "fetch_url":
        return derivation("some custom derivation with a load() statement and everything")
```

That file has load() statements in it (and might be served from the binary). The version of bramble that is selected in the bramble file choses what script is used. The script's load statements pick which version of the fetcher it is. Would want to make this implementation minimal? Just pull something from a URL. Could allow people to override with their own things if ever needed. This does mean that we need to allow the fetch_url builder to have network access, something we have to be careful about.

So does this help? Now we have a baked in derivation. What if someone tries to run this with a different bramble version? The script will run and GC the other derivation. Ok cool.

------------


How does the fetch_url builder return the hash? Will need to read the output? Does that mean we can also do hash checking outside of the derivation? Will mean more internals that could change. But ok, let's start with that.
