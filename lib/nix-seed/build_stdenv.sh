set -e


PATH=$PATH:$patchelf/bin:$busybox/bin

echo $PATH

cp -r $src/* $out
export LD_LIBRARY_PATH=$out/lib

for filename in $out/bin/*; do
    patchelf --shrink-rpath $filename
    patchelf --set-interpreter $out/lib/ld-linux-x86-64.so.2 $filename
    patchelf --set-rpath $out/lib $filename
done

$out/bin/bash --help

for filename in $out/libexec/gcc/x86_64-unknown-linux-gnu/8.3.0/*; do
    # ignore liblto
    if echo "$filename" | grep -q "liblto"; then
        continue
    fi
    patchelf --shrink-rpath $filename
    patchelf --set-interpreter $out/lib/ld-linux-x86-64.so.2 $filename
    patchelf --set-rpath $out/lib $filename
done
