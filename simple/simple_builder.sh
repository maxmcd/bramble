set -uxe
export PATH="$BRAMBLE_PATH/store/$hermes_seed/bin"
echo $PATH
env
mkdir -p $out
gcc --static -o $out/simple $BRAMBLE_PATH/store/$src/simple.c
$out/simple
