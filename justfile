set shell := ["bash", "-euo", "pipefail", "-c"]

# Complete offline gate inside the Fedora build environment.
check: _check-style
    go vet ./...
    go test -tags=integration ./...
    go run ./cmd/coprctl verify-specs

# Run the complete gate without changing host packages.
check-container: image
    docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp -v "$PWD:/work:ro" furyfree-copr-tools just check

# Build the same Fedora tooling environment used in CI.
image:
    docker build -t furyfree-copr-tools -f build/Containerfile .

# Unit tests without RPM tooling.
test:
    go test ./...

# Shared tooling and one package's tests, spec and native RPM checks.
check-package package: _check-style
    #!/usr/bin/env bash
    set -euo pipefail
    package={{ quote(package) }}
    go run ./cmd/coprctl verify-specs "$package"
    targets=(./cmd/coprctl ./internal/packaging)
    case "$package" in
        github-copilot-installer) targets+=(./cmd/github-copilot-installer ./internal/copilot) ;;
        wowup-cf-installer) targets+=(./cmd/wowup-cf-installer ./internal/wowup) ;;
    esac
    go vet "${targets[@]}"
    COPR_TEST_PACKAGE="$package" go test -tags=integration "${targets[@]}" -count=1

[private]
_check-style:
    bash build/check-format.sh
    shellcheck build/*.sh
    git diff --check

# Prepare one source RPM; only source preparation may access the network.
prepare package outdir:
    go run ./cmd/coprctl prepare {{ quote(package) }} {{ quote(outdir) }}

# Publish one package from remote main.
publish package: (_dispatch "build" package)

# Create or verify one COPR project without building.
project package: (_dispatch "project" package)

[private]
_dispatch operation package:
    @case {{ quote(package) }} in nimbus|voxtype|github-copilot-installer|wowup-cf-installer) ;; *) echo 'Choose nimbus, voxtype, github-copilot-installer, or wowup-cf-installer' >&2; exit 2 ;; esac
    gh workflow run copr.yml --repo Furyfree/copr --ref main -f {{ quote("operation=" + operation) }} -f {{ quote("package=" + package) }}
