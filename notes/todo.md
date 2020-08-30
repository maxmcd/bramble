- switch to `bramble foo` run syntax
- figure out how tests work and are imported
- imports are modules with methods, imports are folders not files
- disallow global function calls in scripts, scripts are only run as methods, needed for derivations and bramblescript
- turn tests and seed folder into lib folder and just write tests with the new format

- derivations are a chain of "runnables" these are scripts (command+args) or functions
- we need to explicitly pass other derivations to the derivation
- bramblescript can be used as part of run commands or used as a derivation runnable
