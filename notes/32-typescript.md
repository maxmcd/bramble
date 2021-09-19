would be cool to write lots of build scripts in deno/typescript

Could do this by providing a derivation that is pre-built with deno bits, maybe more practical to use Recursive Builds. Could just have a ts lib that either spawned a bramble binary to run builds or talk to the socket. oh man, bramble could call out to a recursive build socket if it found that it was available within an environment. Cool, should get Go working so that we can compile bramble and start running it within itself.
