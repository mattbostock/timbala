all: test build
.PHONY: build savedeps test

build:
	@go install ./...

savedeps:
	@govendor add +external

test:
	@go test $(shell go list ./... | grep -v /vendor/)
