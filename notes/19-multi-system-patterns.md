Originally the plan was to have a shared bramble store for all system types. Since macos has a case insentive filesystem this almost immediately breaks.

For docker we'll need to mount a store volume. We'll still need to download a bramble binary and copy it into the store. From there we can create the bramble store inside the docker container and copy in that single derivation.

How do we allow users to select a different build target? Maybe docker is just special. We could provide software level built-ins that people could use. For now I feel like it's docker only so we ignore some of the config.
