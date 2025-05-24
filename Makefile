SHELL := /bin/bash

.PHONY: init run

init:
	@if [ ! -f go.mod ]; then \
		module=$$(git remote get-url origin \
			| sed -e 's#git@#https://#' \
				-e 's#:#/#' \
				-e 's#\.git$$##'); \
		go mod init $${module}; \
	fi

run:
	go run main.go