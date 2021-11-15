#!/usr/bin/env bash
set -Eeuxo pipefail
cd "$(dirname ${BASH_SOURCE[0]})"

cd ..

# We run tests twice so that we can generate a project-wide coverage report
# including all tests while still running non-integration tests with -race

# Run all tests with integration tests and collect coverage information
env BRAMBLE_INTEGRATION_TEST=truthy gotestsum -- -coverpkg=./... -coverprofile=coverage.txt -v ./...
# Run non-integration tests with race detection.
gotestsum -- -race -v ./...
bash <(curl -s https://codecov.io/bash)
