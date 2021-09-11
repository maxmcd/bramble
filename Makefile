

test: go_test \
	integration_test

ci_test: go_ci_test \
	integration_ci_test

gotestsum:
	go get gotest.tools/gotestsum

go_ci_test: gotestsum
	gotestsum -- -race -v ./...

go_test:
	go test -race -v ./...

# just use LICENSE as a file we can harmlessly "touch" and use as a cache marker
LICENSE: main.go pkg/*/*.go
	CGO_ENABLED=1 go install
	touch LICENSE

install: LICENSE

integration_ci_test: install gotestsum
	env BRAMBLE_INTEGRATION_TEST=truthy gotestsum -- -v ./pkg/bramble/

integration_test: install
	env BRAMBLE_INTEGRATION_TEST=truthy go test -v ./pkg/bramble/

