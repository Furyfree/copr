# LibrePods for Noctalia

This packages the [harveywuk/librepods fork](https://github.com/harveywuk/librepods)
at commit `4ed49df0b301ac3e6fba9c81dfbbb6726cc52201`, dated 2026-08-26.
The fork has no tagged release, so the RPM uses its snapshot date as the version.
Its source archive is pinned by SHA-256 in the spec. Compilation and the upstream
CTest suite run offline with disposable configuration, state and cache paths.
The bundled QR code library's MIT notice is included alongside the GPL license.

Release 2 applies `librepods-fix-a2dp-profile-selection.patch` to this snapshot.
It identifies playback profiles by their A2DP names and accepts the input source
that WirePlumber can expose alongside stereo playback. This prevents those valid
profiles from being rejected and sent through the daemon's audio recovery path.
The patch includes a sanitized Fedora 44 profile fixture and non-playback
exclusion tests. Remove it when a future pinned source includes the fix.

The fork supplies the status JSON and extended control commands required by
Noctalia's `harveywuk/airpods` widget. It also supplies `--headless`, which creates
neither a tray icon nor the graphical window. The standard LibrePods desktop
entry remains available for deliberate GUI use.

## Build and publish

~~~sh
just prepare librepods /tmp/librepods-srpm
rpmbuild --rebuild /tmp/librepods-srpm/librepods-20260826-2.fc44.src.rpm
just publish librepods
~~~

Source preparation may download the pinned archive. Run the binary rebuild in
the Fedora environment from `build/Containerfile`, with networking disabled.
`publish` is a separate, explicit action against remote main. Adding this recipe
does not enable a repository on a workstation.

## Session startup

The RPM installs `/usr/bin/librepods`, `/usr/bin/librepods-ctl` and
`/usr/lib/systemd/user/librepods.service`. The service preserves the fork's
sandbox settings and private state-directory permissions, uses the packaged
executable, and skips startup when the executable or Bluetooth adapter is absent.
Fedora user-service macros handle installation, upgrade and removal. Installation
does not start the daemon.

Hyprland may start the unit once at session startup after checking that it exists:

~~~sh
systemctl --user start librepods.service
~~~

Use a single startup owner. Do not also enable LibrePods' GUI autostart. A
headless instance cannot display the GUI; stop the user unit first if GUI access
is needed, and start it again after closing the GUI. The service belongs to the
graphical session and restarts on failure.

This package uses the same name and executable paths as Terra's LibrePods and
replaces that implementation. Review the DNF transaction before switching. To
return to the standard package, disable this COPR and use DNF distro-sync for
`librepods`; private device settings are retained. Do not manually delete user
configuration or status files.

Nimbus must select this COPR only after publication and verification of the
actual project signing key. No placeholder key or unpublished repository is
added to Nimbus definitions. Building and testing the RPM does not establish
real AirPods connectivity or validate a live service upgrade.

## Validation

The Fedora 44 tools image built the source RPM and rebuilt it with networking
disabled. All 19 CTest targets passed, including the release 2 profile-selection
regressions. Both new regression cases failed against the unpatched selector
and passed with the patch. The repository's complete
`just check` gate also passed in that image with networking disabled, including
the integration-tagged Go suite and RPM spec validation.

Live Fedora 44 checks with AirPods Pro 2 passed two 45-second pause/resume
cycles, disconnect/reconnect through `librepods-ctl`, automatic microphone
profile selection, preservation of live capture during daemon startup, and
return to SBC-XQ after capture ended. WirePlumber did not restart. Microphone
data was discarded; these checks establish routing, not recorded sound quality.

ANC, Adaptive, Transparency and ear-detection policy commands were acknowledged.
The noise Off command remained at Transparency in the single-earbud test;
the device's separate Off Listening Mode setting was not verified. Physical
ear-removal auto-pause and audible sound quality still require manual testing.
