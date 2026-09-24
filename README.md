# COPR packaging

Fedora 44 x86_64 RPM recipes and installer helpers.

| Package | Purpose |
| --- | --- |
| [blesh](packages/blesh/README.md) | Bash highlighting, suggestions and completion |
| [nimbus](packages/nimbus/README.md) | Fedora workstation installer and system manager |
| [nimbus-develop](packages/nimbus-develop/README.md) | Nimbus development channel |
| [voxtype](packages/voxtype/README.md) | Push-to-talk speech transcription |
| [librepods](packages/librepods/README.md) | AirPods controls with Noctalia integration |
| [woeusb](packages/woeusb/README.md) | Create Windows installation USB media |
| [github-copilot-installer](packages/github-copilot-installer/README.md) | Install the official GitHub Copilot application |
| [wowup-cf-installer](packages/wowup-cf-installer/README.md) | Install WoWUp with CurseForge |
| [jetbrains-toolbox-installer](packages/jetbrains-toolbox-installer/README.md) | Install JetBrains Toolbox |

## Development

Install Go (the version in [go.mod](go.mod) or newer) and Just for unit tests,
or use Docker for the full Fedora checks.

```sh
just test
just check-container
just prepare PACKAGE /tmp/package-srpm
```

`prepare` creates a source RPM for local testing and requires the Fedora tools
in [build/Containerfile](build/Containerfile). Source preparation and the initial
container image build need network access; the full test gate runs offline.
Run `just --list` for all commands.

## Installer release pins

```sh
just check-pins
just refresh-pins
just check-package PACKAGE
git commit -m 'chore(PACKAGE): pin VERSION' internal/pins/pins.json && git push
just publish PACKAGE
```

`check-pins` and `refresh-pins` need network access. `check-pins` reports
outdated releases; `refresh-pins` updates
[internal/pins/pins.json](internal/pins/pins.json) and names the helpers to
check and publish. Each command prints the next one. The weekly Installer pins
workflow also checks for outdated releases.

## Publishing

Configure the GitHub Actions variable `COPR_OWNER` and secret `COPR_CONFIG`
using your [COPR API configuration](https://copr.fedorainfracloud.org/api/).
Authenticate the GitHub CLI, then run:

```sh
just publish PACKAGE
```

Replace `PACKAGE` with a name from the table. Publication uses remote `main`,
checks the selected package, prepares its source RPM, creates or verifies its
COPR project, and submits the build. Use `just project PACKAGE` to create or
verify the project without building. Pushes and pull requests run checks only.

Installer helper builds use the checked-in release pins. They download their
applications on the destination computer; COPR contains only the helpers.
