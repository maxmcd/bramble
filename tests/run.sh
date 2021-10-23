#!/usr/bin/env bash
set -Eeuxo pipefail
cd "$(dirname ${BASH_SOURCE[0]})"

cd ..

env BRAMBLE_INTEGRATION_TEST=truthy gotestsum -- -coverpkg=./... -coverprofile=coverage.txt -v ./...
bash <(curl -s https://codecov.io/bash)
gotestsum -- -race -v ./...
