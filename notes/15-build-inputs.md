built inputs should come back so that disparate libraries can write derivation functions without having to do all sorts of tricks around finding the binary they're wrapping in the path, this also undermines some of the guarantees of the system. If a derivation can point to a binary in its output it should be able to.

linely want to bring back the derivation build_inputs field (more extreme measures could take everything in scope in the file, maybe that would possibly work with incremental building when needed? )
