#!/usr/bin/env bash
set -Eeuxo pipefail
cd "$(dirname ${BASH_SOURCE[0]})"

docker build -t bramble-seed .


id=$(docker create bramble-seed)
docker cp $id:linux-x86_64-seed.tar.gz ./linux-x86_64-seed.tar.gz
docker rm -v $id
