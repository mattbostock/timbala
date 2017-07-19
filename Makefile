all: test build
.PHONY: all build clean integration savedeps servedocs test testdocs

MKDOCS_MATERIAL_VERSION=1.5.4
VERSION := $(shell git describe --always | tr -d '\n'; test -z "`git status --porcelain`" || echo '-dirty')

build:
	@go install -ldflags "-X main.version=$(VERSION)" ./...

clean:
	@docker-compose --file internal/integration/docker-compose.yml rm -f

integration:
	@docker-compose --file internal/integration/docker-compose.yml up --build --abort-on-container-exit --exit-code-from integration_tests

savedeps:
	@govendor add +external

test:
	@go test -race $(shell go list ./... | grep -v /vendor/)

servedocs:
	@docker run --rm -it -p 8000:8000 -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION)

testdocs:
	@docker run --rm -it -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION) build -s
