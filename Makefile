

export BRAMBLE_PATH=$(shell pwd)/brambles


bootstrap_run:
	go run main.go bootstrap-tools.bramble.py
