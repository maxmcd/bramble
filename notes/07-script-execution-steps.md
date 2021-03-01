# Execution steps

- parse input file and crawl all imports, calls to derivation or cmd are blocked in imports
- call a function, execute the scripts and compute all derivations
- when we complete or when we hit the first `cmd()` call we build all parse derivations (some derivations might be created that we don't necessarily need, we could build as we need them?)


so that's the question, what is the entrypoint. I think if a function returns a derivation we should allow various actions on that derivation?

if it's just a cmd() call and we do everything through that then we
