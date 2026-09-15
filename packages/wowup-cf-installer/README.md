# WoWUp CurseForge installer helper

The Go helper and tests live in `cmd/wowup-cf-installer` and `internal/wowup`.
This directory maintains its MIT license, manual and RPM recipe.
The helper originated in Nimbus's local desktop-delivery work; the original
Copyright (c) 2026 Patrick Byrne notice is preserved in `LICENSE`. Builds use
these local sources and do not fetch or redistribute the application.

Published **0.3.0-0.1.app2.23.1** is available for Fedora 44 x86_64 in
[`furyfree/wowup-cf-installer`](https://copr.fedorainfracloud.org/coprs/furyfree/wowup-cf-installer/),
[COPR build 10989485](https://copr.fedorainfracloud.org/coprs/furyfree/wowup-cf-installer/build/10989485/).
It selects official WoWUp-CF **2.23.1** and automatically manages installation
and removal through DNF. The offline Fedora gate, remote CI and COPR build
passed. Installation and removal were also verified on the owner's desktop
with 0.2.0; CurseForge acceptance checks remain pending. Purge was tested using
disposable profiles, including the packaged executable, without removing the
owner's real profile.

## Installation and updates (0.2.0)

~~~sh
sudo dnf copr enable furyfree/wowup-cf-installer
sudo dnf install wowup-cf-installer
~~~

Installing, reinstalling or upgrading the helper automatically queues a one-shot
systemd job to download the official AppImage, verify it and create the launcher
and desktop entry. No second installer command is required. Start **WoWUp with
CurseForge** from the application menu after the job finishes.

DNF completion means the job was queued, not that the app download succeeded.
Inspect completion or a failed download with:

~~~sh
systemctl status wowup-cf-installer.service
journalctl -u wowup-cf-installer.service
wowup-cf-installer status --json
~~~

The job waits for the current DNF transaction, retries failures at 60-second
intervals with at most three starts in 15 minutes, and reports failures through
systemd. After fixing a persistent problem, `sudo dnf reinstall
wowup-cf-installer` queues another attempt. Offline image/chroot installation
cannot start the job; reinstall the helper after booting with systemd running.
This is not a boot-enabled service or a periodic updater.

The signed helper package selects an application version and SHA-256. Publishing
an updated selection advances the RPM release, so `sudo dnf upgrade` triggers
the next application update. WoWUp's own application updater stays disabled;
addon updates remain inside WoWUp. `wowup-cf-installer release --json` reports
the selected release offline. A fully verified current install needs no download
or replacement; missing known desktop links can be repaired from the retained
artifact while offline. A failed download or extraction preserves the previous
installation. Unknown or modified files block replacement.

The helper owns the app lifecycle outside RPM's file inventory; COPR contains
only the open-source installer and release metadata.

~~~sh
sudo dnf remove wowup-cf-installer
~~~

Final package removal first stops the installer job and its pending retries,
then removes the verified app bundle, launcher and desktop entry. Upgrades and
reinstalls do not remove the app. If stopping the job, verifying ownership or
cleaning up fails, RPM refuses to erase the helper so removal can be retried
after resolving the problem. Interrupted cleanup resumes from its ownership
journal. Home directories, application settings, WoW installations and addons
are always retained. The standalone `uninstall --assumeyes` command remains
available for explicit app-only removal.

### Purge personal data (0.3.0)

Version 0.3.0 adds the following explicit cleanup command. Close WoWUp first,
then run as your normal user, **without sudo**:

~~~sh
wowup-cf-installer purge --assumeyes
sudo dnf remove wowup-cf-installer
~~~

Run purge before removing the helper package, while its command is still
available. It deletes your `$XDG_CONFIG_HOME/WowUpCf` profile (normally
`~/.config/WowUpCf`), including preferences, saved sessions/cookies, logs and
caches. It does not uninstall the shared application, delete other users'
profiles, or touch WoW installations and addons outside this profile. You can
also use it alone to reset WoWUp. Normal DNF removal still preserves user data.

Purge requires `--assumeyes`, refuses root execution, and never uses
`SUDO_USER` to select another account. It rejects a profile that is itself a
symlink, foreign-owned entries, special files, and crossings to another
filesystem. Symlinks inside the profile are removed without following their
targets. A live or unrecognised Electron session lock blocks cleanup; a stale
local lock whose process no longer exists is accepted. Keep WoWUp closed during
purge. An interrupted or failed purge may have removed some files; fix the
reported problem and rerun it. An already absent profile is a successful no-op.
Success prints JSON containing `schema_version`, `name`, `path` and `purged`.

Nimbus may install this package through its normal DNF flow. Existing
prepare/apply/status/uninstall interfaces and receipts remain compatible.
Standalone `install --assumeyes` and `update --assumeyes` reconcile the
package-selected release, but are not required for normal package installation.

The owner approved the official CurseForge AppImage with GitHub's SHA-256 and
an exception limited to this artifact source. These checks identify the
approved official bytes; they are not an independent publisher signature.
The helper accepts only published stable numeric versions, the exact versioned
`WowUp/WowUp.CF` asset URL, HTTPS redirects within GitHub, and an x86_64 type-2
AppImage. It does not extract credentials or execute the downloaded runtime.

## Preparation and approval

~~~sh
wowup-cf-installer prepare --directory /existing/private/staging
# Approve the returned version, source and SHA-256 before applying.
sudo wowup-cf-installer apply --appimage /returned/path/to/app.AppImage \
  --sha256 EXPECTED_SHA256 --app-version SELECTED_VERSION --assumeyes
wowup-cf-installer status --json
sudo wowup-cf-installer uninstall --assumeyes
~~~

`prepare --directory DIR [--app-version VERSION]` runs without root. The
existing directory receives a private child containing the verified download.
Success prints one JSON object with `schema_version: 1`, `name: wowup-cf`,
`arch: x86_64`, `version`, `sha256`, `path`, and `source`. Failed preparation
cleans its child; successful preparation leaves cleanup to the caller.
The GitHub metadata size must be an integer between 64 bytes and 1 GiB.
Downloads are bounded to that exact size and reject short or excess content.

Nimbus must show the artifact and complete operation before approval. The
helper's `apply --appimage PATH --sha256 HEX --app-version VERSION
--assumeyes` copies into private privileged staging, rechecks its hash and
AppImage identity, and refuses version downgrades. `--assumeyes` is required
for mutation; it records caller intent, not a substitute for Nimbus approval.

Extraction uses native `unsquashfs` at the validated ELF section-table end.
The downloaded runtime is never run for extraction. Files have ordinary root
ownership and modes; no setuid permissions, device nodes or external symlinks
are accepted. Application launch preserves Chromium's sandbox.

## Owned paths and updates

The helper owns this fixed bundle:

- `/opt/wowup-cf/WowUp-CF.AppImage`: approved original artifact.
- `/opt/wowup-cf/app/`: extracted application.
- `/opt/wowup-cf/launcher`: launch wrapper.
- `/opt/wowup-cf/wowup-cf.desktop`: desktop entry.
- `/opt/wowup-cf/receipt.json`: version, source, artifact hash and the complete
  owned file inventory, including content hashes and modes.

It also owns two exact root-owned symlinks:

- `/usr/local/bin/wowup-cf` points to the bundle's `launcher`.
- `/usr/local/share/applications/wowup-cf.desktop` points to the bundle's
  desktop entry.

Missing parent directories are created with root ownership and mode `0755`.
Empty parent directories remain after removal. Foreign entries, unsafe parent
ownership, parent symlinks, modified payloads and unknown bundle files block
replacement or deletion. The helper never removes anything below the home
directory during app installation or uninstall, and does not update desktop
caches as an incidental operation. Only explicit user-scoped purge removes the
user's WoWUp profile.

The complete bundle is synced and published using Linux `renameat2`; existing
bundle updates exchange directories atomically. Global link creation is a
separate operation. If link creation fails, the verified bundle remains and
another approved apply repairs missing links. This is not a claim that the
whole install or removal is atomic or that failed operations rolled back.
A killed process or filesystem failure can leave a private staging directory;
that requires inspection rather than deletion based only on its name.

`uninstall --assumeyes` verifies the complete owned bundle and global links,
then removes the links. An early link-removal failure keeps the bundle and its
receipt for retry. Before moving the bundle, it persists the validated receipt
to `/opt/.nimbus-wowup-cf-removal.json`, then renames the bundle to
`/opt/.nimbus-wowup-cf-removing`. Both paths belong in the removal preview.
Only recorded entries are deleted. Missing entries are accepted during removal
retry, while unknown or changed surviving entries block cleanup. The external
journal is removed last, after the entire directory is gone, so even a failure
removing the final directory retains durable ownership for another uninstall.
Unknown ownership always blocks removal.

## Status and application updates

`status --json` is offline and performs no writes. It returns `schema_version`,
`name`, `installed`, `verified`, `integrated`, and `cleanup_pending`. A verified
installation also returns `version`, `sha256`, `path` and `source`. Missing
known integration links produce `integrated: false`, which is repairable. Invalid or foreign state
returns an error; it is never reported as a successful absent installation.
With `cleanup_pending: true`, the original identity represents an owned
installation still being removed, not a launchable application. `verified`
then means the remaining state matches the durable removal journal, and
`integrated` is false. Only resuming removal is allowed; a selected application
must finish that removal before reinstalling. The helper receipt does not
replace Nimbus's operation receipt.

The launcher sets `APPDIR` and clears `APPIMAGE`, `APPIMAGE_EXIT_AFTER_INSTALL`
and `DEBUG` before invoking the extracted `AppRun`. This prevents the official
Electron updater from treating the launch as a self-updating AppImage and
prevents AppRun's debug mode from printing the environment. Addon updates
remain the application's responsibility. The upstream desktop entry's
`--no-sandbox` flag is deliberately not used.

## Verification

Run the isolated fixture gate:

~~~sh
go test ./internal/wowup
~~~

Tests inject private paths, native operations and ownership expectations into
disposable fixtures. Production has no environment variable or command-line
override for system paths or root ownership. A checked-in receipt from the
original Python implementation verifies on-disk compatibility. Tests exercise
native directory exchange,
preview/download boundaries, foreign ownership, tampering, failed updates,
incomplete desktop integration, removal retry and user-data preservation.
They do not install or launch the application on the host.

Native extraction was also verified against the official 2.23.1 AppImage in
an unprivileged, network-disabled Fedora 44 container: ELF/SquashFS offset
188392, 176 files, 47 directories, two internal symlinks and no special files.
The archive SHA-256 was
`8d74c8fd24af4904dfd47b608ea6f4c55d578fd9dd70ad176b81f7edf139c7d9`.
Fedora's signature-verified `squashfs-tools-4.6.1-8.fc44.x86_64` and
`lzo-2.10-16.fc44.x86_64` RPMs were unpacked into temporary tooling storage,
without package installation. This caught an unsupported extraction flag;
the helper now uses that Fedora version's native listing preflight and
strict-error extraction.

Real desktop launch, working Chromium sandbox, CurseForge addon search and
downloads, and absence of app-update downloads remain installation acceptance
checks. A passing ownership test does not establish those runtime results.

## Package build and publication

Run the repository's `just check` in its network-disabled Fedora tooling
container. It builds this package's SRPM and binary RPM, runs all helper
fixtures, checks the payload, version, dependency declarations and MIT notice,
and verifies the deferred installer scriptlets and service. It also tests
package-pinned updates, offline reruns, rejected release changes and retries.
A real systemd-host install/upgrade/retry drill remains an acceptance check.

To retain a source RPM with the native tooling available:

~~~sh
just prepare wowup-cf-installer /tmp/wowup-cf-installer-srpm
~~~

After review and merge, `just publish wowup-cf-installer` resolves the latest
stable official release metadata and builds the helper with that version and
SHA-256. It does not download or bundle the application on COPR. Local
`prepare` uses the checked-in release pin; add `--latest` to the `coprctl
prepare` command to resolve current metadata without publishing. Source
preparation bundles this helper and its required Go modules; binary builds are
offline. The installed helper needs no Python runtime or upstream API credential.
Update the Go helper version constant, spec and manual together. See
[publishing](../../README.md#publishing).

## Why the application is downloaded separately

On 2026-09-10, inspected upstream `v2.23.1`, commit
`e7b7538e54dae5516ff54c8e4fe95315ab22865c`, in `WowUp/WowUp`. That repository
builds both editions; the `WowUp.CF` repository publishes the CF release assets.

- The [production configuration][config] contains a CurseForge API-key
  placeholder. The [provider][provider] passes it directly to `curseforge-v2`,
  including on Linux. There is no separate keyless Linux provider path here.
- The [official CF workflow][workflow] replaces that placeholder using its
  `CURSEFORGE_API_KEY` secret. A third-party source build does not obtain that
  credential from the public source archive.
- [CurseForge documents][api] authenticated API access. Read-only requests to
  `/v1/mods/search?gameId=1&pageSize=1` returned HTTP 403 both without a key and
  with the source placeholder. These responses do not by themselves identify
  every rejection cause; the source and API contract establish the missing
  prerequisite.
- An offline Fedora 44 tooling container confirmed version 2.23.1 and the
  placeholder in the source. Its package definition selects Overwolf Electron
  39.8.10 for CF. Compilation and an RPM build were not attempted after this
  prerequisite failed; a successful compilation would not prove usable
  CurseForge access.

The official application is downloaded from its publisher at explicit user
request. A source RPM for the application can be reconsidered if upstream
provides a supported distribution arrangement or suitable API access. Public
SRPMs and binary builds must never contain private credentials.

[config]: https://github.com/WowUp/WowUp/blob/v2.23.1/wowup-electron/src/environments/environment.prod.ow.ts
[provider]: https://github.com/WowUp/WowUp/blob/v2.23.1/wowup-electron/src/app/addon-providers/curse-addon-provider.ts
[workflow]: https://github.com/WowUp/WowUp/blob/v2.23.1/.github/workflows/electron-ow-all-build.yml
[api]: https://docs.curseforge.com/rest-api/
[release]: https://github.com/WowUp/WowUp.CF/releases/tag/v2.23.1
[updater]: https://github.com/WowUp/WowUp/blob/v2.23.1/wowup-electron/app/app-updater.ts
