with import <nixpkgs> { };
stdenv.mkDerivation {
  name = "hello";
  src = ./main.go;
  buildInputs = [ pkgs.go ];
  unpackPhase = "true";
  installPhase = ''
    set -x
    mkdir -p "$out/bin"
    HOME=$(pwd)
    go run $src
    set +x
  '';
}
