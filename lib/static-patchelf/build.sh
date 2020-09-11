#!/usr/bin/env bash
set -Eeuxo pipefail
cd "$(dirname ${BASH_SOURCE[0]})"

docker build -t static-patchelf .


id=$(docker create static-patchelf)
docker cp $id:/usr/local/bin/patchelf.tar.gz patchelf.tar.gz
docker rm -v $id
