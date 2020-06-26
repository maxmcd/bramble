

export BRAMBLE_PATH=$(shell pwd)/brambles


seed_run:
	go run main.go hermes-seed.bramble.py

simple_run:
	go run main.go ./simple/simple.bramble.py
