all: test build
.PHONY: build savedeps test

build:
	@go install -tags=embed ./...

savedeps:
	@govendor add +external

test:
	@go test -tags=embed $(shell go list ./... | grep -v /vendor/)
