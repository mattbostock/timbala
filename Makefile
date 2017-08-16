all: test build
.PHONY: all build clean integration savedeps servedocs test testdeps testdocs

MKDOCS_MATERIAL_VERSION = 1.5.4
VERSION = $(shell git describe --always | tr -d '\n'; test -z "`git status --porcelain`" || echo '-dirty')
UNUSED_LIBS = $(shell govendor list +unused)

build:
	@go install -ldflags "-X main.version=$(VERSION)" ./...

clean:
	@docker-compose --file internal/test/integration/docker-compose.yml rm -f

integration:
	@docker-compose --file internal/test/integration/docker-compose.yml up --build --abort-on-container-exit --exit-code-from integration_tests

savedeps:
	@govendor add +external

test:
	@go test -race $(shell go list ./... | grep -v /vendor/)

servedocs:
	@docker run --rm -it -p 8000:8000 -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION)

testdeps:
	@govendor status
	@test -z "$(UNUSED_LIBS)" || ( echo "Libraries vendored but unused:\n$(UNUSED_LIBS)" && exit 1 )

testdocs:
	@docker run --rm -it -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION) build -s
