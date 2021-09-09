

test: go_test \
	integration_test

test_sandbox:
	cd pkg/sandbox/cmd/libcontainer/ && go run .

ci_test: go_ci_test \
	integration_ci_test \
	test_sandbox

gotestsum:
	go get gotest.tools/gotestsum

go_ci_test: gotestsum
	gotestsum -- -race -v ./...

go_test:
	go test -race -v ./...

# just use LICENSE as a file we can harmlessly "touch" and use as a cache marker
LICENSE: main.go pkg/*/*.go
	go install
	make build_setuid
	touch LICENSE

install: LICENSE

integration_ci_test: install gotestsum
	env BRAMBLE_INTEGRATION_TEST=truthy gotestsum -- -v ./pkg/bramble/

integration_test: install
	env BRAMBLE_INTEGRATION_TEST=truthy go test -v ./pkg/bramble/

build_setuid:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -tags netgo -ldflags '-w' ./pkg/cmd/bramble-setuid
	sudo chown root:root ./bramble-setuid
	sudo chmod u+s,g+s ./bramble-setuid
	rm -f $$(go env GOPATH)/bin/bramble-setuid || true
	mv ./bramble-setuid $$(go env GOPATH)/bin
