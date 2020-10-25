Looked into how we might generate code for language-specific projects. Initial showstopper was that Go projects expect to be able to pull git repositories. That is a relatively complex network action that can't be handled with fetch_url. It seems like we have 2-ish options here:

1. Start building out the various network fetching builtings we would need and support them all
2. Allow certain derivations to use the network.

#1 is appealing but ends up stifling creativity. Only the magic project maintainer can add your new networked runtime.

#2 is ok, but we need to figure out how to apply pressure so that derivations using the network expect a fixed hashed output and are never(!) re-downloaded after the first downloads.

Could also mount a cache directory to derivations. If we're going to allow some networked derivations we might also want to allow caches. As long as builds are deterministic this is kind of ok? Agh, maybe not.

I guess, how can we ensure that a networked build is protected by an output hash of some kind? Maybe our general work in ensuring builds are deterministic is helped by this on some level?
