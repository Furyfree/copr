# COPR packaging

Fedora 44 x86_64 packages and installer helpers.

| Package | Purpose |
| --- | --- |
| [blesh](packages/blesh/README.md) | Bash highlighting, suggestions and fzf completion |
| [nimbus](packages/nimbus/README.md) | Nimbus engine |
| [voxtype](packages/voxtype/README.md) | Voxtype speech-to-text daemon |
| [librepods](packages/librepods/README.md) | Headless AirPods daemon with Noctalia integration |
| [woeusb](packages/woeusb/README.md) | Create bootable Windows USB installation media |
| [github-copilot-installer](packages/github-copilot-installer/README.md) | Automatically install the Copilot release selected by the package |
| [wowup-cf-installer](packages/wowup-cf-installer/README.md) | Install and update the official WoWUp CurseForge AppImage |
| [jetbrains-toolbox-installer](packages/jetbrains-toolbox-installer/README.md) | Install and update the official JetBrains Toolbox App without autostart |

Installer helpers do not bundle their applications. Copilot, WoWUp and JetBrains
Toolbox automatically queue the selected app's download and installation when
the helper package is installed or upgraded.
See each package's README for installation, status and release details.

## Development

Tools and tests use Go, with Bash for small wrappers. Package recipes live in
`packages/`; shared development rules are in [AGENTS.md](AGENTS.md).

```sh
just check-container                        # full offline gate; requires Docker
just test                                   # unit tests; requires Go 1.26.7+
just check-pins                             # report stale installer pins; needs network
just prepare voxtype /tmp/voxtype-srpm        # optional local source RPM; requires Go and Fedora build tools
```

Local checks and CI share [build/Containerfile](build/Containerfile).
`prepare` is for local testing and does not publish anything.
Run `just --list` for all commands.

`just check-pins` resolves the latest stable upstream release for Copilot,
WoWUp and JetBrains Toolbox and reports how each checked-in pin differs; it
reads metadata only and never publishes. The scheduled "Installer pins"
workflow runs the same check weekly and fails while a pin is stale, so a
forgotten refresh becomes visible. The check makes unauthenticated GitHub API
requests, and GitHub disables scheduled workflows after 60 days without
repository activity; dispatch the workflow manually if it has gone quiet.

Refresh a stale pin by updating `internal/<implementation>/release.json`
(version, source, sha256) and the spec's `%global app_version`, then commit and
publish. `just publish PACKAGE` resolves the latest release for the SRPM, but
native COPR builds through `.copr/Makefile` use the checked-in pin.

## Publishing

Configure the GitHub Actions variable `COPR_OWNER` and secret `COPR_CONFIG`
using your [COPR API configuration](https://copr.fedorainfracloud.org/api/).

```sh
just publish voxtype                        # publish one package
```

`publish` prepares the source RPM, creates/verifies the COPR project, and builds
and publishes the package. You do not need to run `prepare` or `project` first.
For Copilot, WoWUp and JetBrains Toolbox, publication resolves the latest stable
upstream release and embeds its version and SHA-256 in the helper. The app version advances the RPM
release, so DNF sees a package update. Application artifacts are downloaded only
on the computer that installs the helper.
Use `just project voxtype` only to create/verify the project without a build.

Replace `voxtype` with any package listed above. Publishing requires an
authenticated GitHub CLI and uses remote `main`. Pushes and pull requests run
full checks only; publication is manual and checks shared tooling plus the selected package.
