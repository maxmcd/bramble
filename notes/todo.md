 - might want to actually pass dependencies to the derivation
 - I think we also need to merge environment variables for dependencies?


- derivations are a chain of "runnables" these are scripts (command+args) or functions
- we need to explicitly pass other derivations to the derivation
- bramblescript can be used as part of run commands or used as a derivation runnable
