TARGETS := $(shell ls scripts)

.dapper:
	@echo Downloading dapper
	@curl -sL https://github.com/pspdfkit-ops/dapper/releases/download/v0.3.4-PSPDFKit-1.0.0/dapper-`uname -s`-`uname -m` > .dapper.tmp
	@@chmod +x .dapper.tmp
	@./.dapper.tmp -v
	@mv .dapper.tmp .dapper

$(TARGETS): .dapper
	./.dapper $@

release: .dapper
	./.dapper $@

.DEFAULT_GOAL := ci

.PHONY: $(TARGETS)
