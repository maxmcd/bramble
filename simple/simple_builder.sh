set -u
export PATH="$seed/bin"
mkdir $out
gcc -o $out/simple $src
