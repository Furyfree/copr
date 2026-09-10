# Voxtype

Published for Fedora 44 x86_64, built from the signed upstream 1.0.1 source.
It builds the CPU Whisper daemon with no optional GPU/ONNX/OSD features.
Nimbus has not selected it yet. Models, languages, hotkeys, and optional
acceleration remain deployment choices.

`prepare.py` verifies the reviewed source SHA-256 and upstream detached
signature against `signing.asc`, then vendors the exact Cargo.lock dependencies.
The key fingerprint is `9CCF7915B750CAE8B095ED1AA3FC9F33FD209279`, identified by
[the tagged upstream release workflow](https://github.com/peteonrails/voxtype/blob/v1.0.1/.github/workflows/build-linux.yml).
The generated SRPM includes a digest-bound source bundle; binary compilation
runs offline. The checked-in spec intentionally refuses unprepared source.
Version bumps must review the new source digest, signing identity, dependencies,
license inventory, and native build before publication.

Use a Fedora container with the RPM tools, Cargo, Rust, GnuPG, C/C++ compiler,
Clang development files, ALSA development files, CMake, and systemd RPM macros:

```sh
python3 packages/voxtype/prepare.py --outdir /tmp/voxtype-srpm
rpmbuild --rebuild /tmp/voxtype-srpm/voxtype-1.0.1-0.2.fc44.src.rpm
```

Source preparation accesses the network; run the subsequent binary build with
networking disabled. Neither command publishes to COPR. After merging the recipe,
use the manual COPR workflow with `package=voxtype` and `operation=build`.
It runs this preparation and uploads only the Voxtype SRPM to `<owner>/voxtype`.
The `project` operation can create/verify that project without building.
See the [publishing workflow](../../README.md#workflow).

The package supplies the executable, user unit, config, completions, and manual
pages. First installation applies Fedora/admin user-service presets; Fedora
44 leaves Voxtype disabled, and installation does not start it. The package
does not create input-group membership or download a model. The prepared
source removes the upstream unit's unconditional
`Wants=ydotool.service`; compositor keybindings do not require that backend.
Configuration still needs review before enabling the daemon. Upstream's
multi-binary post-install CPU/GPU selector is not used: no maintainer script
replaces files in `/usr/bin` on install/removal.

Release 0.2 fixes the service lifecycle (P2) with Fedora's native user-unit
macros. On removal, the pre-uninstall hook disables the unit globally and
disables/stops it in running user managers. On upgrade, the old package's
post-uninstall hook marks the unit for restart; systemd's transaction triggers
reload user managers and process those markers. User configuration and models
are retained. Initial activation still requires setup and an explicit user
choice. A restart can interrupt an in-progress recording.

The repository gate exercises the expanded native scriptlets for first
installation, upgrade, and removal, including missing/failing helper cases.
It also checks Fedora's actual preset policy using `systemctl --root` against
a temporary filesystem. These checks do not operate a live user manager;
the real session install/update/removal trial remains a deployment gate.

Cargo dependency license declarations were inventoried with the default Linux
normal/build dependency graph; license notices are included in the package.
Native microphone, transcription, compositor output, service start/stop, and
install/update/removal trials remain separate from compilation and CLI smoke
checks. Confirm CPU compatibility and any additional acceleration features on
the actual target before deployment.

Local validation on 2026-09-09 passed a Fedora 44 x86_64 offline rebuild,
including 1,002 upstream library tests and the CLI version/help checks. Two
upstream model-download tests remain ignored. The test build needed a 10 GiB
container memory limit; the initial 6 GiB limit caused memory pressure.
The then-current `make check` passed all 11 repository tests and both package
spec checks. The current local gate is `just check`.

Published on 2026-09-10 as `voxtype-1.0.1-0.2.fc44.x86_64` in
[COPR build 10968949](https://copr.fedorainfracloud.org/coprs/furyfree/voxtype/build/10968949/).
The offline COPR build passed 1,002 upstream library tests; the two
model-download tests remained ignored. The downloaded RPM's service
scriptlets were checked, and its signature and digests verified against the
[project key](https://download.copr.fedorainfracloud.org/results/furyfree/voxtype/pubkey.gpg),
fingerprint `B4DED69E7793D5DF2FC32BC1C69E64492CA2E833`.
