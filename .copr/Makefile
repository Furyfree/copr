# COPR's make_srpm interface supplies spec and outdir in the package directory.
export spec outdir
srpm_helper := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/../scripts/srpm.py)

.PHONY: srpm
srpm:
	python3 "$(srpm_helper)" --spec "$$spec" --outdir "$$outdir"
