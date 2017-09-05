all: build test
.PHONY: all bench build clean integration savedeps servedocs test testdeps testdocs

MKDOCS_MATERIAL_VERSION = 1.5.4
VERSION = $(shell git describe --always | tr -d '\n'; test -z "`git status --porcelain`" || echo '-dirty')

bench:
	@docker-compose --file internal/test/bench/docker-compose.yml up --build

build:
	@CGO_ENABLED=0 go install -ldflags "-X main.version=$(VERSION)" ./cmd/athensdb/

clean:
	@docker-compose --file internal/test/bench/docker-compose.yml rm -f
	@docker-compose --file internal/test/integration/docker-compose.yml rm -f

integration:
	@docker-compose --file internal/test/integration/docker-compose.yml up --build --abort-on-container-exit

savedeps:
	@dep ensure

test:
	@go vet ./...
	@go test ./internal/cluster
	@go test -race ./...

servedocs:
	@docker run --rm -it -p 8000:8000 -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION)

testdeps:
	@dep status 1>/dev/null

testdocs:
	@docker run --rm -it -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION) build -s
