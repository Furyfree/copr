# JetBrains Toolbox installer helper

The Go helper and tests live in `cmd/jetbrains-toolbox-installer` and
`internal/toolbox`. This directory maintains its MIT license, manual and RPM
recipe. Builds use these local sources and do not fetch or redistribute the
application.

Local candidate **0.1.0-0.1.app3.8.0.87909** selects official JetBrains Toolbox
**3.8.0.87909** and automatically manages installation and removal through DNF.
It has not been published to COPR yet. The offline fixture gate passes; the
container gate, a real systemd-host install/upgrade/removal drill and a real
desktop launch from the bundle remain acceptance checks.

## Installation and updates

~~~sh
sudo dnf copr enable furyfree/jetbrains-toolbox-installer
sudo dnf install jetbrains-toolbox-installer
~~~

Installing, reinstalling or upgrading the helper automatically queues a one-shot
systemd job to download the official Linux archive, verify it and create the
launcher and desktop entry. No second installer command is required. Start
**JetBrains Toolbox** from the application menu after the job finishes, or run
`jetbrains-toolbox` from a terminal.

DNF completion means the job was queued, not that the app download succeeded.
Inspect completion or a failed download with:

~~~sh
systemctl status jetbrains-toolbox-installer.service
journalctl -u jetbrains-toolbox-installer.service
jetbrains-toolbox-installer status --json
~~~

The job waits for the current DNF transaction, retries failures at 60-second
intervals with at most three starts in 15 minutes, and reports failures through
systemd. After fixing a persistent problem, `sudo dnf reinstall
jetbrains-toolbox-installer` queues another attempt. Offline image/chroot
installation cannot start the job; reinstall the helper after booting with
systemd running. This is not a boot-enabled service or a periodic updater.

The signed helper package selects an application build and SHA-256. Publishing
an updated selection advances the RPM release, so `sudo dnf upgrade` triggers
the next application update. `jetbrains-toolbox-installer release --json`
reports the selected release offline. A fully verified current install needs no
download or replacement; missing known desktop links can be repaired from the
retained archive while offline. A failed download or extraction preserves the
previous installation. Unknown or modified files block replacement.

~~~sh
sudo dnf remove jetbrains-toolbox-installer
~~~

Final package removal first stops the installer job and its pending retries,
then removes the verified app bundle, launcher and desktop entry. Upgrades and
reinstalls do not remove the app. If stopping the job, verifying ownership or
cleaning up fails, RPM refuses to erase the helper so removal can be retried
after resolving the problem. Interrupted cleanup resumes from its ownership
journal. Home directories, installed IDEs, projects and Toolbox settings are
always retained. The standalone `uninstall --assumeyes` command remains
available for explicit app-only removal.

## No autostart

Toolbox enables "Launch at system startup" by default. On every launch it
reconciles `~/.config/autostart/jetbrains-toolbox.desktop` from the `autostart`
value in `~/.local/share/JetBrains/Toolbox/.settings.json` (or the same path
under `$XDG_DATA_HOME`): it creates the entry when the value is true and deletes
it when the value is false. A system package cannot change per-user settings at
install time, so the owned launcher records the opt-out at launch instead:

- If the user's `.settings.json` does not exist, the launcher creates it with
  `{"autostart": false}` and then starts Toolbox.
- An existing `.settings.json` is never modified. Toolbox owns the file; the
  user can enable autostart later in Toolbox settings, and Toolbox keeps that
  choice.

Both the desktop entry and `/usr/local/bin/jetbrains-toolbox` go through the
launcher. Starting `/opt/jetbrains-toolbox/app/bin/jetbrains-toolbox` directly
before the first launch skips the seed, and Toolbox then applies its default.

This behaviour was verified against the official 3.8.0.87909 archive in a
disposable home directory using `jetbrains-toolbox --integrate-into-system`,
which runs the startup integration and exits without opening a window. Without
a settings file, Toolbox created the autostart entry. With the seeded file, it
created none, deleted a pre-existing stale entry, honoured `XDG_DATA_HOME` and
preserved `"autostart": false` when it rewrote the file.

## Self-update

Toolbox updates itself by installing a new build next to the running one.
Inside the root-owned `/opt/jetbrains-toolbox/app` that write cannot succeed,
so Toolbox may report a failed self-update and postpone it; application updates
arrive through new helper packages instead. No newer Toolbox build existed
while this package was prepared, so that failure path has not been observed in
practice. IDE installation and IDE updates happen entirely under the user's
home directory and are unaffected.

## Preparation and approval

~~~sh
jetbrains-toolbox-installer prepare --directory /existing/private/staging
# Approve the returned build, source and SHA-256 before applying.
sudo jetbrains-toolbox-installer apply --archive /returned/path/to/app.tar.gz \
  --sha256 EXPECTED_SHA256 --app-version SELECTED_BUILD --assumeyes
jetbrains-toolbox-installer status --json
sudo jetbrains-toolbox-installer uninstall --assumeyes
~~~

`prepare --directory DIR [--app-version BUILD]` runs without root. The existing
directory receives a private child containing the verified download. Success
prints one JSON object with `schema_version: 1`, `name: jetbrains-toolbox`,
`arch: x86_64`, `version` (the four-component build number), `sha256`, `path`
and `source`. Failed preparation cleans its child; successful preparation
leaves cleanup to the caller. Release metadata comes from JetBrains' public
releases service (`data.services.jetbrains.com`, product code `TBA`, type
`release`); the digest comes from the `.sha256` file published beside the
archive, and the download must be exactly the advertised size. Downloads are
accepted only from `download.jetbrains.com` and its `download-cdn` redirect,
over HTTPS. Early-access builds are refused.

`apply --archive PATH --sha256 HEX --app-version BUILD --assumeyes` copies into
private privileged staging, rechecks the hash and gzip identity, and refuses
build downgrades. `--assumeyes` is required for mutation. The archive is
unpacked by the helper itself: exactly one top-level directory is required and
stripped; only directories, regular files and relative symlinks that stay
inside the application are accepted; hard links, device nodes, absolute paths,
traversal and duplicate entries are rejected; symlinks are created only after
every file, so nothing is written through a link; files receive ordinary root
ownership and `0644`/`0755` modes. Nothing from the archive is executed.

## Owned paths and updates

The helper owns this fixed bundle:

- `/opt/jetbrains-toolbox/jetbrains-toolbox.tar.gz`: approved original archive.
- `/opt/jetbrains-toolbox/app/`: unpacked distribution; the executable is
  `app/bin/jetbrains-toolbox` with its bundled runtime beside it.
- `/opt/jetbrains-toolbox/launcher`: launch wrapper described above.
- `/opt/jetbrains-toolbox/jetbrains-toolbox.desktop`: desktop entry.
- `/opt/jetbrains-toolbox/receipt.json`: build, source, archive hash and the
  complete owned file inventory, including content hashes and modes.

It also owns two exact root-owned symlinks:

- `/usr/local/bin/jetbrains-toolbox` points to the bundle's `launcher`.
- `/usr/local/share/applications/jetbrains-toolbox.desktop` points to the
  bundle's desktop entry. Toolbox writes its own entry with the same name under
  `~/.local/share/applications` on first launch, which takes precedence for
  that user and points at the same bundle.

Missing parent directories are created with root ownership and mode `0755`.
Empty parent directories remain after removal. Foreign entries, unsafe parent
ownership, parent symlinks, modified payloads and unknown bundle files block
replacement or deletion. The helper never removes anything below the home
directory.

The complete bundle is synced and published using Linux `renameat2`; existing
bundle updates exchange directories atomically. Global link creation is a
separate operation. If link creation fails, the verified bundle remains and
another approved apply repairs missing links. `uninstall --assumeyes` verifies
the complete owned bundle and global links, removes the links, persists the
validated receipt to `/opt/.jetbrains-toolbox-removal.json`, renames the bundle
to `/opt/.jetbrains-toolbox-removing` and deletes only recorded entries. The
journal is removed last, so a failure at any point can be retried. Unknown
ownership always blocks removal.

## Status

`status --json` is offline and performs no writes. It returns `schema_version`,
`name`, `installed`, `verified`, `integrated`, and `cleanup_pending`. A verified
installation also returns `version`, `sha256`, `path` and `source`. Missing
known integration links produce `integrated: false`, which is repairable.
Invalid or foreign state returns an error; it is never reported as a successful
absent installation. With `cleanup_pending: true`, only resuming removal is
allowed.

## Verification

Run the isolated fixture gate:

~~~sh
go test ./internal/toolbox
~~~

Tests build small gzip archives with the official layout and inject private
paths and ownership expectations into disposable fixtures. Production has no
environment variable or command-line override for system paths or root
ownership. Tests exercise real extraction, including rejection of absolute
paths, traversal, escaping and absolute symlinks, writes through a symlinked
directory, hard links, device nodes, duplicate and multiple top-level entries;
native directory exchange; preview/download boundaries; foreign ownership;
tampering; failed updates; incomplete desktop integration; removal retry; user
data preservation; and the launcher's settings seeding, argument forwarding and
`XDG_DATA_HOME` handling (with `shellcheck` when available). They do not
install or launch the application on the host.

The extractor was also run once against the official 3.8.0.87909 archive
(`3bd93bc3d251dc142514fda1e9aba9d2768f8cb84f17b7599befa001466af1cf`) in a
temporary directory: 917 archive entries produced 710 files, 138 directories
and 68 internal symlinks below the stripped top-level directory, and the
inventory accepted the tree. The distribution
ships its own Java runtime; the launcher, runtime and native libraries link
only against system libraries that the package requirements declare or pull in
transitively, as checked with `ldd` on Fedora 44.

Real desktop launch from `/opt`, IDE installation through Toolbox, and the
self-update failure path remain installation acceptance checks. A passing
ownership test does not establish those runtime results.

## Package build and publication

Run the repository's `just check` in its network-disabled Fedora tooling
container. It builds this package's SRPM and binary RPM, runs all helper
fixtures, checks the payload, version, dependency declarations and MIT notice,
and verifies the deferred installer scriptlets and service.

~~~sh
just prepare jetbrains-toolbox-installer /tmp/jetbrains-toolbox-installer-srpm
~~~

After review and merge, `just publish jetbrains-toolbox-installer` resolves the
latest stable official release metadata and builds the helper with that build
number and SHA-256. It does not download or bundle the application on COPR.
Local `prepare` uses the checked-in release pin; add `--latest` to the
`coprctl prepare` command to resolve current metadata without publishing.
Update the Go helper version constant, spec and manual together.
See [publishing](../../README.md#publishing).

## Why the application is downloaded separately

JetBrains distributes the [Toolbox App](https://www.jetbrains.com/toolbox-app/)
as a proprietary binary under its own user agreement; it is not open source and
may not be redistributed through a third-party repository. The official
archive is therefore downloaded from JetBrains on the computer that installs
the helper, at that computer's request, and verified against the SHA-256 that
JetBrains publishes beside it. Public SRPMs and binary builds never contain the
application.
