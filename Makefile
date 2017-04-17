all: test build
.PHONY: build savedeps servedocs test

MKDOCS_MATERIAL_VERSION=1.5.4

build:
	@go install ./...

savedeps:
	@govendor add +external

servedocs:
	@docker run --rm -it -p 8000:8000 -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION)

test:
	@go test $(shell go list ./... | grep -v /vendor/)
	@docker run --rm -it -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION) build -s
