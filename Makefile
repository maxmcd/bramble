

gopath := $(shell go env GOPATH)

test: go_test \
	integration_test

ci_test: install gotestsum
	bash ./tests/run.sh


go_test: gotestsum
	gotestsum -- -race -v ./...

install:
	go mod tidy
	go install

gotestsum: $(gopath)/bin/gotestsum
$(gopath)/bin/gotestsum:
	go install gotest.tools/gotestsum@latest
	go mod tidy

build: install
	bramble build ./...

integration_test: install
	env BRAMBLE_INTEGRATION_TEST=truthy gotestsum -- -run=$(run) -v ./internal/command/

rootless_within_docker:
	docker build -t bramble . && docker run --privileged -it bramble bramble build ./lib:busybox

upload_url_fetcher:
	cd cmd/url_fetcher && make upload

cover:
	env BRAMBLE_INTEGRATION_TEST=truthy go test -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	rm coverage.out

