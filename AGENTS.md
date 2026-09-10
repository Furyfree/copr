# COPR packaging

This repository owns Fedora RPM recipes, release tooling, and the Copilot and
WoWUp installer helpers. Nimbus and Voxtype application source stays upstream.

- Use Go for maintained tools, installers and tests. Keep Bash limited to
  command wrappers, launchers and RPM build steps. Follow the module's Go version.
- Keep command entry points in `cmd/`, implementation and adjacent tests in
  `internal/`, and recipes, package documentation and static files in `packages/`.
- Preserve installer command lines, JSON schemas, owned paths and on-disk
  receipts when changing implementations. Unknown ownership blocks removal.
- Use native RPM, DNF, COPR, Cargo, GnuPG and SquashFS tools for their respective
  operations. Do not implement a substitute package manager.
- Preparation may download reviewed sources. Binary RPM builds and the complete
  test gate run offline. Source bundles include only declared source and licenses.
- `just check` is the full gate in the Fedora build environment;
  `just check-container` runs it without changing host packages. Tests must use
  disposable paths and must never mutate the host installation or user state.
- `just prepare PACKAGE OUTDIR` creates one SRPM. `just publish PACKAGE` dispatches
  remote main for that package only. Commit, push, publication, installation and
  privileged host operations require explicit user authorization.
- Keep `build/Containerfile` as the shared local/CI tooling definition. Preserve
  `.copr/Makefile` as the native COPR adapter and `justfile` as the user interface.
- Treat existing worktree changes as intentional. Run focused regressions and
  the full gate; inspect the final diff and report limitations honestly.

## Code Review Rules

Review source trust, credential handling, ownership, failure recovery, command
compatibility and actual RPM contents. Documentation commands are behavior.
Fixture success does not prove real desktop, microphone or sandbox operation.
