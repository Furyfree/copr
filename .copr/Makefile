# COPR's make_srpm interface supplies spec and outdir in the package directory.
export COPR_SPEC := $(abspath $(spec))
export COPR_OUTDIR := $(abspath $(outdir))
repo := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/..)

.PHONY: srpm
srpm:
	cd "$(repo)" && GOTOOLCHAIN=local go run ./cmd/coprctl srpm --spec "$$COPR_SPEC" --outdir "$$COPR_OUTDIR"
