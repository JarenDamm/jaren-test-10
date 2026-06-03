# Owner module: language-go-http
#
# Convenience targets for the generated service. This file is managed by the
# language module — re-apply refreshes it.
.PHONY: build test lint run tidy

build:
	go build ./...

test:
	go test ./...

lint:
	golangci-lint run ./...

run:
	go run ./cmd/server

tidy:
	go mod tidy
