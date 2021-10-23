#!/usr/bin/env bash
set -Eeuxo pipefail
cd "$(dirname ${BASH_SOURCE[0]})"

cd ..

gotestsum -- \
    -coverpkg=./... -coverprofile=coverage.txt -covermode=atomic -race -v ./...
bash <(curl -s https://codecov.io/bash)

env BRAMBLE_INTEGRATION_TEST=truthy gotestsum -- \
    -coverpkg=./... -coverprofile=coverage.txt -v ./internal/command/
bash <(curl -s https://codecov.io/bash)
