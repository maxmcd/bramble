

Run as docker 3 ways:
 - explicitly run a derivation with docker as the system
 - pass a derivation (or many) into a docker image build builtin
 - pass the --system=docker command line argument

For local build:
 - mount the build with ~/bramble/store at a fixed path
 - don't run as root
 - build somewhere outside of the store but with the same path length?
 - copy the result out of the container? oh, no, could just write it to another volume
 - yeah, build somewhere that's not the store
