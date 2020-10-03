

seed/linux-x86_64-seed.tar.gz:
	./seed/build.sh

test: go_test bramblescripts_to_test seed simple drv_test bramble_tests

go_test:
	go test -race -v ./...
	go test -run="(TestIntegration|TestRunAlmostAllPublicFunctions)"unique -v ./...


install:
	go install

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
	bramble run tests/starlark-builder:run_busybox

simple: install
	bramble run tests/simple/simple:simple

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

test_starlark_builder:
	go test -v -run=TestStarlarkBuilder ./pkg/bramble/

test_nix_seed:
	go test -v -run=TestNixSeed ./pkg/bramble/

test_simple:
	go test -v -run=TestSimple ./pkg/bramble/

seed: install
	bramble run lib/seed:seed

all_bramble: install
	bramble run all:all

bb2: install
	bramble run all:bb2

install_reptar:
	cd pkg/reptar/reptar && go install
