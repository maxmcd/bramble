

test: go_test \
	integration_test

go_test: install
	go test -race -v ./...

# just use LICENSE as a file we can harmlessly "touch" and use as a cache marker
LICENSE: main.go pkg/*/*.go
	go install
	touch LICENSE

install: LICENSE build_setuid

integration_test: install
	env BRAMBLE_INTEGRATION_TEST=truthy go test -v ./pkg/bramble/

build_setuid:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -tags netgo -ldflags '-w' ./pkg/cmd/bramble-setuid
	sudo chown root:root ./bramble-setuid
	sudo chmod u+s,g+s ./bramble-setuid
	rm -f $$(go env GOPATH)/bin/bramble-setuid || true
	mv ./bramble-setuid $$(go env GOPATH)/bin
