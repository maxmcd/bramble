## Performance


- **Source files need to be hashed and copied into every build**: Can use tricks like `git` does where we check mod times and re-use previous source artifacts if files haven't changed. Can also start using complex FS layers to efficiently story many slightly different versions of the same source.
- **bramble run github.com/*/* is slow**
    - Since we can't pass arguments that effect builds to bramble run we can cache almost everything, just keep a mapping between the run command and the output.
    - `bramble run` might be slow because we have to wait for all files to download. We could mount a FUSE filesystem, download the file list first, start the process and then prioritize file downloads based off of `open()` calls.
- **`bramble run` locally has to parse all source files before running**: Can cache the output of `bramble run`, we keep a mapping between the `bramble run` outputs and their invocations. For in-project `run` invocations we can parse local files only if they have changed and check to confirm that `bramble.lock` has not been updated with new dependencies (also with mod time).
- **Hashing everything takes time**: ????
- **The store fills up with duplicate files**: Can explore filesytem layers that make storage more compact. Also as long as we always run things with `bramble run` we can do strange things like JIT dependency unpacking.
