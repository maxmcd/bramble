with (import <nixpkgs> { });
derivation {
  name = "simple";
  builder = "${bash}/bin/bash";
  args = [ ./simple_builder.sh ];
  inherit gcc coreutils;
  src = ./simple;
  system = builtins.currentSystem;
}
