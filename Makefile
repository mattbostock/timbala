all: test build
.PHONY: all build clean integration savedeps servedocs test testdocs

MKDOCS_MATERIAL_VERSION=1.5.4

build:
	@go install ./...

clean:
	@docker-compose --file integration_tests/docker-compose.yml rm -f

integration:
	@docker-compose --file integration_tests/docker-compose.yml up --build --abort-on-container-exit --exit-code-from integration-tests

savedeps:
	@govendor add +external

test:
	@go test $(shell go list ./... | grep -v /vendor/)

servedocs:
	@docker run --rm -it -p 8000:8000 -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION)

testdocs:
	@docker run --rm -it -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION) build -s
