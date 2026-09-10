# COPR packaging

Fedora 44 x86_64 packages and installer helpers.

| Package | Purpose |
| --- | --- |
| [nimbus](packages/nimbus/README.md) | Nimbus engine |
| [voxtype](packages/voxtype/README.md) | Voxtype speech-to-text daemon |
| [github-copilot-installer](packages/github-copilot-installer/README.md) | Automatically install the Copilot release selected by the package |
| [wowup-cf-installer](packages/wowup-cf-installer/README.md) | Install and update the official WoWUp CurseForge AppImage |

Installer helpers do not bundle their applications. Copilot package installation
and upgrades automatically queue the selected app's download and installation;
the WoWUp helper still requires explicit invocation.
See each package's README for installation, status and release details.

## Development

Tools and tests use Go, with Bash for small wrappers. Package recipes live in
`packages/`; shared development rules are in [AGENTS.md](AGENTS.md).

```sh
just check-container                        # full offline gate; requires Docker
just test                                   # unit tests; requires Go 1.26.7+
just prepare voxtype /tmp/voxtype-srpm        # optional local source RPM; requires Go and Fedora build tools
```

Local checks and CI share [build/Containerfile](build/Containerfile).
`prepare` is for local testing and does not publish anything.
Run `just --list` for all commands.

## Publishing

Configure the GitHub Actions variable `COPR_OWNER` and secret `COPR_CONFIG`
using your [COPR API configuration](https://copr.fedorainfracloud.org/api/).

```sh
just publish voxtype                        # publish one package
```

`publish` prepares the source RPM, creates/verifies the COPR project, and builds
and publishes the package. You do not need to run `prepare` or `project` first.
For Copilot, publication resolves the latest stable GitHub release and embeds
its version and SHA-256 in the helper. The app version advances the RPM release,
so DNF sees a package update. The proprietary RPM is downloaded only on the
computer that installs the helper.
Use `just project voxtype` only to create/verify the project without a build.

Replace `voxtype` with any package listed above. Publishing requires an
authenticated GitHub CLI and uses remote `main`. Pushes and pull requests run
full checks only; publication is manual and checks shared tooling plus the selected package.
