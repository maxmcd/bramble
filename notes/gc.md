- derivations are left on disk
- derivations can be fetched by hash and name
- outputs can be fetched, this is safe because the derivation contains the system name

So if we copied something into a docker container it would be similar to how we run starlark scripts. We'd copy in the parsed starlark files and the entire chain of derivations. Nothing gets built, but whatever gets run has the files it needs on disk.

Consider packing starlark files into a binary executables (how to trim execution paths?)
