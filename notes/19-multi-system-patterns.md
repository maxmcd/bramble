Originally the plan was to have a shared bramble store for all system types. Since macos has a case insentive filesystem this almost immediately breaks.

For docker we'll need to mount a store volume. We'll still need to download a bramble binary and copy it into the store. From there we can create the bramble store inside the docker container and copy in that single derivation.

How do we allow users to select a different build target? Maybe docker is just special. We could provide software level built-ins that people could use. For now I feel like it's docker only so we ignore some of the config.


------------------

Lots has changed since the above.

We need to support:

- Builds on this system for this system
- Builds on this system cross-compiled to another system
- Builds on this system using a builder that is available to this system but not the default

Examples:

1. Cross-compile a go project to another language. On a linux system pass a `system` variable of `x86_64-darwin` and the go project handles? Or do we leave this interface up to implementors? Cross compiling natively in the platform means that we're aware of where the output can run. So could allow a cross-compiled binary to be used on the correct system... hm.
2. Partial builds in one system and one in the other? What would need this? A build script to produce all outputs, would probably just do them one by one? I think this is ok.
3. It's still a bummer that you can't implement a system that defines different weights to where things should be built. Maybe we can't support that yet.

(these aren't examples...)


Ok, so we just go with `platform` and we expect builders to pass a new `platform` for cross-compile situations. That way we could hmm... no, how does that work, I want to compile on linux and then pass a different platform to the final build. Ah, yes, and then `go get github.com/maxmcd/foo` would just need to need to return the final derivation? But then how does it know to request the right build environment. And how do I keep the same api with different outputs? We need to define builder and system. builder is the builder, system is the output system. could system be set to anything? yeah this is good, because then the same build tree can be used to cross-compile different outputs for different platforms, and we can make sure all the right shared libs are along for the ride.....

I hope this makes sense in the morning.
