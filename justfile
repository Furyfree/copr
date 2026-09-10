set shell := ["bash", "-euo", "pipefail", "-c"]

# Complete offline gate inside the Fedora build environment.
check:
    bash build/check-format.sh
    shellcheck build/*.sh
    go vet ./...
    go test -tags=integration ./...
    go run ./cmd/coprctl verify-specs
    git diff --check

# Run the complete gate without changing host packages.
check-container: image
    docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp -v "$PWD:/work:ro" furyfree-copr-tools just check

# Build the same Fedora tooling environment used in CI.
image:
    docker build -t furyfree-copr-tools -f build/Containerfile .

# Unit tests without RPM tooling.
test:
    go test ./...

# Native package checks, including helper RPM builds.
check-package package:
    COPR_TEST_PACKAGE={{ quote(package) }} go test -tags=integration ./internal/packaging -count=1
    go run ./cmd/coprctl verify-specs {{ quote(package) }}

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
