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
just refresh-pins                           # write latest stable installer pins to internal/pins/pins.json
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

Installer application versions live in `internal/pins/pins.json`. That is the
only file a pin bump should edit. `just refresh-pins` fills it from the latest
stable upstream releases, including source URLs and SHA-256 digests. Specs
read the version at SRPM preparation. `just publish PACKAGE` builds the helper
from that file; native COPR rebuilds use the same pin.

## Publishing

Configure the GitHub Actions variable `COPR_OWNER` and secret `COPR_CONFIG`
using your [COPR API configuration](https://copr.fedorainfracloud.org/api/).

```sh
just publish voxtype                        # publish one package
```

`publish` prepares the source RPM, creates/verifies the COPR project, and builds
and publishes the package. You do not need to run `prepare` or `project` first.
For Copilot, WoWUp and JetBrains Toolbox, publication embeds the application
release from `internal/pins/pins.json`. The app version advances the RPM
release, so DNF sees a package update. Application artifacts are downloaded only
on the computer that installs the helper.
Use `just project voxtype` only to create/verify the project without a build.

Replace `voxtype` with any package listed above. Publishing requires an
authenticated GitHub CLI and uses remote `main`. Pushes and pull requests run
full checks only; publication is manual and checks shared tooling plus the selected package.
