

bramblescript_shell:
	go run . script

seed_run:
	go run . run seed:seed

simple_run:
	go run . run tests/simple/simple:simple

seed/linux-x86_64-seed.tar.gz:
	./seed/build.sh

test:
	go test -v ./...

install:
	go install

bramble_tests: install
	bramble test ./tests

docker_reptar:
	cd pkg/reptar && docker build -t reptar . \
	&& docker run -it reptar sh

bramblescripts_to_test: install
	bramble run pkg/bramble/cmd-examples:main

drv_test: install
	bramble test tests/derivation_test.bramble


starlark_builder: install
	bramble run tests/starlark-builder:run_busybox
