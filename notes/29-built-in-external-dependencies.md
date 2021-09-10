
We should remove mholt/achiver and any other built-in software that we should more reasonably. Now that build and project are separatated we can just define the derivations we would need in the input format that the build expects. We can feed in derivation inputs and then write our source into the store (if needed). These derivations can download programs like git and tar at fixed versions. These dependencies will show up in the bramble.toml file so that a project can be guaranteed to see the same versions of git/tar/whatever to rebuild the software that they've fetched


We can even have the derivation for older versions of bramble. When a project needs an old version of bramble we just store all old versions of bramble to download and run (this is trivial now that we don't need root). Project specifies a specific version of bramble we download and run that instead (another reason to keep the binary small). Ah, could even keep libcontainer just in a sandbox utility that we download to run builds. Oof, interesting idea.


Write a tool that takes starlark derivations and turns them into go code. Generate the resulting derivations and then output go code with the structs.


