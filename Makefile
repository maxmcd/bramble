

seed/linux-x86_64-seed.tar.gz:
	./seed/build.sh

test: go_test bramblescripts_to_test seed simple drv_test

go_test:
	go test -v ./...

install:
	go install

bramble_tests: install
	bramble test ./tests

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

gc: install
	bramble gc

go: install
	bramble run lib/go:go

seed: install
	bramble run lib/seed:seed

all_bramble: install
	bramble run all:all

bb2: install
	bramble run all:bb2
