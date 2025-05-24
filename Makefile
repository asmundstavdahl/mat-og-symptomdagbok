SHELL := /bin/bash

TMPDIR := $(CURDIR)/.tmp
export TMPDIR

.PHONY: init run

init:
	mkdir -p .tmp .githooks
	@git config core.hooksPath .githooks
	@if [ ! -f go.mod ]; then \
		module=$$(git remote get-url origin \
			| sed -e 's#git@#https://#' \
				-e 's#:#/#' \
				-e 's#\.git$$##'); \
		go mod init $${module}; \
	fi

run:
	mkdir -p .tmp
	go run main.go