# GitHub Copilot installer helper

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

Local Go candidate 0.2.0 adds separate preparation and application commands,
stable-release and downgrade checks, and a GitHub HTTPS redirect allowlist.
`status` remains offline with no temporary files. Termination signals clean up
incomplete downloads and privileged staging. The tests cover these paths. The helper supports
Fedora 43/44 x86_64; our tested packaging target is Fedora 44 x86_64.

## Application lifecycle

The `install` and `update` commands download Copilot directly from GitHub.
`--app-version VERSION` selects a release; otherwise an explicit invocation
resolves the latest release using GitHub's API. There is no hardcoded app
version and no timer or background service. `status` reports only the helper
and installed app versions, without checking for new releases.

COPR/DNF updates the helper. An explicit helper `update` command updates the
application; upgrading the helper alone does not upgrade Copilot. `uninstall`
removes the native `github` RPM and preserves user data. Removing only the
helper leaves Copilot installed. The SRPM and binary RPM contain no
proprietary app payload and have no installation scriptlets or triggers.

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

Nimbus's integration plan is in its
[roadmap](https://github.com/Furyfree/nimbus/blob/main/docs/ROADMAP.md). The helper
provides download and transaction primitives; Nimbus still owns its review,
approval, receipts, retry and removal workflow. Dotfiles has no system-package
role here. Standalone install/update remain available and combine download
with a native DNF confirmation unless `--assumeyes` is supplied.

## Build and verify

Run `just check-container` for the complete offline Fedora gate, or
`just check-package github-copilot-installer` inside that tooling environment.
The gate builds the helper SRPM and x86_64 binary RPM, runs its Go regression
tests, and checks payload, version, licenses and absence of scriptlets/triggers.
No live application transaction is run.

~~~sh
just prepare github-copilot-installer /tmp/github-copilot-installer-srpm
~~~

Preparation bundles only this helper and its tests, with any needed Go modules.
The binary build runs offline; installed runtime dependencies are DNF and RPM.
The helper needs no Python, jq or curl. Update its Go version constant, manual
and spec together. Retain the upstream MIT attribution when modifying the port.

After review and merge, `just publish github-copilot-installer` publishes this
helper only. Verify its signing key before Nimbus integration. The published
0.1.2 RPM remains the Bash implementation until the Go candidate is explicitly
published. See the [publishing workflow](../../README.md#workflow).
