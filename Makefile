

seed/linux-x86_64-seed.tar.gz:
	./seed/build.sh

test: seed go_test bramblescripts_to_test \
	simple drv_test bramble_tests \
	test_integration

go_test: install
	go test -race -v ./...
	go test -run="(TestIntegration|TestRunAlmostAllPublicFunctions)"unique -v ./...


generate_proto: ./pkg/bramblepb/bramble_pb.pb.go

./pkg/bramblepb/bramble_pb.pb.go: ./pkg/bramblepb/bramble_pb.proto
	cd pkg/bramblepb && go generate

# just use LICENSE as a file we can harmlessly "touch" and use as a cache marker
LICENSE: main.go pkg/*/*.go
	go install
	mkdir -p ~/bramble/var
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' . && mv bramble ~/bramble/var/linux-binary
	touch LICENSE

install: ./pkg/bramblepb/bramble_pb.pb.go LICENSE

bramble_tests: install
	bramble test

docker_reptar: ## Used to compare reptar output to gnutar
	cd pkg/reptar && docker build -t reptar . \
	&& docker run -it reptar sh

bramblescripts_to_test: install
	bramble run pkg/bramble/cmd-examples:main

drv_test: install
	bramble test tests/derivation_test.bramble

starlark_builder: install
	bramble run lib/busybox:run_busybox

touch_file: install
	bramble run lib/busybox:touch_file

simple: install
	bramble run tests/simple/simple:run_simple

simple2: install
	bramble run tests/simple/simple:simple2

nested: install
	bramble run tests/nested-sources/another-folder/nested:nested

ldd: install
	bramble run lib/seed:ldd

repl: install
	bramble repl

gc: install
	bramble gc

go: install
	bramble run lib/go:go

delete_store:
	rm -rf ~/bramble

test_integration:
	go test -v -run=TestIntegration ./pkg/bramble/

nix_seed: install
	bramble run lib/nix-seed:stdenv

seed: install
	bramble run lib/seed:seed

all_bramble: install
	bramble run all:all

bb2: install
	bramble run all:bb2

install_reptar:
	cd pkg/reptar/reptar && go install
