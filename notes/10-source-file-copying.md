
- hash all sources before copying them
- check if we have those sources
- copy the sources in to the store into a single directory
- write the source directory to the derivation
- are you allowed to use sources that are outside of the project? if no, why?

- when you build copy all the sources back into a tmpdir and run the build from there
- if this is a problem we can optimize the use-case of minmially copying files within an active project
- how does this look on the derivation? Inputsrc points to the store and has an entrypoint location?

------------

- You could mount the sources into the container instead of copying files? Namespace filtering rules might be sufficient to show the correct files.
- For ^ you'd have to be able to re-assemble the source for uploads, could always re-compute though.
- Could have a cache hash like git, with mod time to confirm that a file doesn't need to be re-hashed.
