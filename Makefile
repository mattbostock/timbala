.PHONY: servedocs testdocs

MKDOCS_MATERIAL_VERSION = 1.5.4

servedocs:
	@docker run --rm -it -p 8000:8000 -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION)

testdocs:
	@docker run --rm -it -v `pwd`:/docs squidfunk/mkdocs-material:$(MKDOCS_MATERIAL_VERSION) build -s
