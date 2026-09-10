# GitHub Copilot package installer

Install or upgrade `github-copilot-installer` with DNF to automatically install
the Copilot release selected by that package. The proprietary RPM is downloaded
directly from GitHub on your computer; COPR hosts only the open-source installer
and its pinned release metadata.

~~~sh
sudo dnf install github-copilot-installer
sudo dnf upgrade github-copilot-installer
~~~

DNF queues a one-shot systemd job. The job waits for DNF's transaction lock,
verifies the download and installs the native `github` RPM. DNF finishing the
helper transaction does not mean the app job has finished. Check its result:

~~~sh
systemctl status github-copilot-installer.service
github-copilot-installer status
~~~

Failures appear in the service status and journal. The job retries after one
minute, with at most three starts in fifteen minutes. It does not poll for new
upstream versions or run at every boot. After a network failure, interrupted
boot or failed job, `sudo dnf reinstall github-copilot-installer` queues it
again. Installing into an image without running systemd prints this same
recovery instruction. A small installer and service remain installed.

This repository maintains the Go installer in `cmd/github-copilot-installer`
and `internal/copilot`, with its recipe, license and manual in this directory. The original 0.1.2 import was derived from
[ChrisTitusTech/fedora-copilot](https://github.com/ChrisTitusTech/fedora-copilot)
0.1.1, imported from commit `be172d54dd20e79f99e625156cc8754d24975e7e`.
The imported archive's SHA-256 was
`5ffdc2b0e74c9308f420961dbe1fbce477cc6a825df7b6b6fd00af96051e34b7`.
The original MIT copyright and permission notice are retained in `LICENSE`.
Future changes are maintained here; builds do not fetch the upstream helper.

Published for Fedora 44 as `github-copilot-installer-0.1.2-0.1.fc44.noarch`
on 2026-09-10 in [COPR build 10968946](https://copr.fedorainfracloud.org/coprs/furyfree/github-copilot-installer/build/10968946/).
The downloaded RPM's signature and digests were verified against the
[project key](https://download.copr.fedorainfracloud.org/results/furyfree/github-copilot-installer/pubkey.gpg),
fingerprint `D09200FC482801551973B03F4A868CE1F2B0A28A`.

Local Go candidate 0.3.0 adds package-triggered installation and pins the
selected app release. It retains separate preparation and application commands,
stable-release and downgrade checks, and a GitHub HTTPS redirect allowlist.
`status` remains offline with no temporary files. Termination signals clean up
incomplete downloads and privileged staging. The tests cover these paths. The helper supports
Fedora 43/44 x86_64; our tested packaging target is Fedora 44 x86_64.

## Application lifecycle

The `install` and `update` commands default to the version and SHA-256 embedded
in the installed helper. GitHub metadata must still match that pin. Publishing
a new Copilot helper selects the latest stable release at preparation time;
new upstream releases do not change existing packages. `--app-version VERSION`
retains the explicit manual override, verified against GitHub's release API.
The standalone `prepare` command still defaults to GitHub's latest stable
release. `status` reports the helper, selected app version and digest, and
installed app version without checking for new releases.

Installing, reinstalling or upgrading the helper queues the app job. `uninstall`
removes the native `github` RPM and preserves user data. Removing only the
helper stops its job and leaves Copilot installed. To remove both through DNF,
remove `github-copilot-installer` and `github` together. The SRPM and binary RPM
contain no proprietary app payload. RPM scriptlets only manage and queue the
service; they do not download files or nest a DNF installation.

The helper checks the official asset URL, GitHub API SHA-256, RPM internal
digests, and native identity, including an epoch of zero. Only published stable
releases are accepted. HTTP redirects must stay on the GitHub API, official
repository release path, or `release-assets.githubusercontent.com`, over HTTPS.
`rpm --checksig --nosignature` checks internal digests, not publisher signatures.
The owner has accepted this trust exception specifically for Copilot. A single
DNF install invocation sets `localpkg_gpgcheck=0` for the verified local Copilot
RPM; repository signature verification and persistent DNF configuration remain
unchanged. Downgrades, including RPM release downgrades, are refused.

For a caller such as Nimbus, separate preparation from approved mutation:

~~~sh
github-copilot-installer prepare --directory /existing/private/staging
# Review the returned version, source and SHA-256 before approving installation.
sudo github-copilot-installer apply --rpm /returned/path/to/app.rpm \
  --sha256 EXPECTED_SHA256 --app-version SELECTED_VERSION --assumeyes
~~~

`prepare` needs no root access and makes no package changes. It creates a
private child of the existing directory, fetches and verifies the artifact,
then writes only JSON to stdout:

~~~json
{
  "schema_version": 1,
  "version": "1.1.16",
  "name": "github",
  "arch": "x86_64",
  "sha256": "<64 lowercase hexadecimal characters>",
  "path": "/existing/private/staging/github-copilot-installer.random/GitHub-Copilot-linux-x64.rpm",
  "source": "https://github.com/github/app/releases/download/v1.1.16/GitHub-Copilot-linux-x64.rpm"
}
~~~

`--app-version VERSION` also works with prepare. Failed preparation removes
its temporary child. Successful preparation retains the artifact; the caller
owns its cleanup. `apply` is offline, requires root, refuses symlink inputs,
and copies the input into private privileged staging before rechecking the
expected digest and native identity. DNF receives only that private copy,
so replacing the original after verification cannot change installed bytes.
The native installed SHA-256 RPM header must match the verified artifact after
DNF. An existing same-version package with a different header is refused before
DNF, since an install no-op would not establish artifact identity. Header
comparison identifies the recorded RPM, not the integrity of live app files.
Both preparation and application refuse a recognized installed newer version.
The caller must approve the exact digest and version before invoking apply.

Nimbus can install this package through its ordinary DNF flow; no separate
postinstall invocation is required. Older Nimbus and Topgrade helper calls
remain supported and default to the package's selected release. Dotfiles has
no system-package role here. Standalone install/update retain native DNF
confirmation unless `--assumeyes` is supplied. The automatic job supplies that
flag because installing the helper requests installation of its selected app.

## Build and verify

Run `just check-container` for the complete offline Fedora gate, or
`just check-package github-copilot-installer` inside that tooling environment.
The gate builds the helper SRPM and x86_64 binary RPM, runs its Go regression
tests, and checks payload, versions, licenses, the systemd unit, deferred
scriptlets and native DNF-compatible locking. No live app transaction or
systemd-host install/upgrade/retry drill is run; those require a disposable VM.

~~~sh
just prepare github-copilot-installer /tmp/github-copilot-installer-srpm
~~~

Local preparation uses the checked-in release pin. To select the latest stable
release without publishing, use:

~~~sh
go run ./cmd/coprctl prepare github-copilot-installer /tmp/copilot-latest --latest
~~~

Preparation bundles only this helper, its release metadata and tests, with
any needed Go modules. It does not download the app RPM.
The binary build runs offline; runtime dependencies include DNF, RPM, systemd
and util-linux's native lock command.
The helper needs no Python, jq or curl. Update its Go version constant, manual
and spec together. The app version is also part of the RPM release. Retain the
upstream MIT attribution when modifying the port.

After review and merge, `just publish github-copilot-installer` prepares this
helper with the latest stable app pin and publishes the helper only. Verify its
signing key before Nimbus integration. The published
0.1.2 RPM remains the Bash implementation until the Go candidate is explicitly
published. See [publishing](../../README.md#publishing).
