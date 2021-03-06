set -ex


export PATH=$stdenv/bin:$busybox/bin
export LD_LIBRARY_PATH=$stdenv/lib

mkdir -p /var/tmp

cp -r $go1_4/go .
cd ./go/src
ls -lah ./cmd/dist
export GO_CCFLAGS="-I$stdenv/include-glibc"
export CC="gcc -I$stdenv/include-glibc -Wl,-rpath=$stdenv/lib "
export GOROOT_BOOTSTRAP=$(pwd)
export CGO_ENABLED="0"
which sh
sed -i 's/set -e/set -ex/g' ./make.bash
# cat ./make.bash
bash ./make.bash
