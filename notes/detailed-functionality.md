# Bramble

Everything starts with a bramblepy file. The root bramblepy file will have no imports and just a derivative that downloads a file. The file is downloaded and optionally decompressed. The file (or directory) is hashed. When a file is hashed the filename of the file is hashed along with the body of the file. Directories are walked (with the contents sorted) and each filename+body is hashed.

From here the derivation file is written, we add the output directory hash and hash the contents. The file is hashed name+body, like everything else. Downloaded files end up in directories with the hash of their contents.

We return a symlinked folder to the output folder.

So in this model all content is just the hash of the name and the content of the file, no complexity. (not the derivation, that must exclude the ouput so that we can check in advance if we have a derivation).


Now you write a bramblepy file that depends on that file. If we ever ask the new file to build it scans the imports and finds the files we depend on. If the file points to a build we have to build that derivation. We build the derivation just from the input file so that we can see if it already exists on our system or if we can fetch it from a cache.

If we don't have the derivation we must build it. We then carry the output forward.

How do we know the origin of the files? If I have a derivation output can it find its dependents? We list:
- output hashed folders
- derivations and their outputs that we depend on while running?
- inputSrcs (files included in the build)


nix heal could find content that doesn't hash correctly and report and rebuild it!! (this doesn't work without full source tracking, which we don't have)

should track the derivations source file, can find everything from there. (and all parent derivations??) this is tough with local builds though, do we track the file in the nix store? I think we ignore how the drv got there for now



