# COPR packaging

Fedora RPM recipes, publication tooling and the Copilot, WoWUp and Toolbox
installer helpers. Other application source stays upstream.

## What must not break

- Preserve installer command lines, JSON schemas, owned paths and receipts.
  Unknown ownership blocks replacement or removal.
- Keep maintained tools and tests in Go, following `go.mod`. Bash is for
  wrappers, launchers and RPM build steps.
- Use native RPM, DNF, COPR, Cargo, GnuPG and SquashFS operations.
- Preparation may download reviewed sources. Binary builds and the full
  test gate run offline. Source bundles contain declared source and licenses.

## Map

- `cmd/`: change command-line entry points here.
- `internal/copilot/`, `internal/wowup/`, `internal/toolbox/`: change
  application delivery and lifecycle behaviour, with adjacent tests.
- `internal/pins/pins.json`: update installer application versions and hashes.
- `internal/packaging/`: change source preparation, pin refresh and publication.
- `packages/`: change RPM recipes, static files and package descriptions.
  Bump helper versions in their Go constants, specs and manuals together.
- `.copr/projects.toml`: configure COPR projects and build targets.
- `.copr/Makefile`: preserve the native COPR source-RPM interface.
- `justfile`: change repository commands.
- `build/Containerfile`: change the shared local and CI build environment.
- `.github/workflows/`: change validation, publication and scheduled pin checks.

## Ways to hurt yourself

- `just publish PACKAGE` dispatches remote `main`; local edits are not published.
- Installer mutations target system files. Tests must use disposable paths and
  never change host packages, installed applications or user state.
- Preserve package and bundled-source license notices.

## Verify

Run focused regressions and `just check-container`, or `just check` inside the
Fedora tooling environment. Inspect source trust, credential handling, ownership,
failure recovery, command compatibility and actual RPM contents. Fixture checks
do not establish real desktop, microphone or sandbox behaviour.

Keep READMEs to usage and brief package descriptions; do not add release history
or verification logs.
