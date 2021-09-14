

test: go_test \
	integration_test

ci_test: go_ci_test \
	integration_ci_test
	make build

gotestsum:
	go get gotest.tools/gotestsum

go_ci_test: gotestsum
	gotestsum -- -race -v ./...

go_test:
	go test -race -v ./...

install:
	go install

build: install
	bramble build

integration_ci_test: install gotestsum
	env BRAMBLE_INTEGRATION_TEST=truthy gotestsum -- -v ./pkg/bramble/

integration_test: install
	env BRAMBLE_INTEGRATION_TEST=truthy go test -run=$(run) -v ./pkg/bramble/

rootless_within_docker:
	docker build -t bramble . && docker run --privileged -it bramble bramble build ./lib:busybox
