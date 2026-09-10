# GitHub Copilot installer helper

This repository maintains the installer script, man page, tests, and RPM
recipe together in this directory. Version 0.1.2 is a local derivative of
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

Our changes make `status` offline with no temporary files, restrict download
redirects to HTTPS, and exit on termination signals after cleanup. The tests
cover these paths and the inherited installation checks. The helper supports
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
digests, and native identity. Its inherited `rpm --checksig --nosignature`
check is not publisher-signature verification. Nimbus has not accepted a
digest-only trust exception and must not invoke installation until that
source-trust decision is resolved.

Nimbus's integration plan is in `~/git/nimbus/docs/ROADMAP.md`. It still needs
reviewed version/checksum selection, approval bound to exact downloaded
bytes, receipts, retry, removal, and downgrade refusal. Install/update still
combine download and mutation. This local import does not implement the
Nimbus provider. Dotfiles has no system-package role here.

## Build and verify

Run the repository's `just check` in its Fedora tooling container. The gate
builds the helper SRPM and binary RPM from these local files, runs its mocked
installer tests, and checks the resulting payload, version, MIT notice, and
empty scriptlet/trigger lists. No live application transaction is run.

To retain a source RPM, from the repository root:

~~~sh
mkdir -p /tmp/github-copilot-installer-srpm
docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp \
  -v "$PWD:/work:ro" \
  -v /tmp/github-copilot-installer-srpm:/output \
  furyfree-copr-tools python3 scripts/srpm.py \
  --spec packages/github-copilot-installer/github-copilot-installer.spec \
  --outdir /output
~~~

Both source and binary builds work offline. Update the version in the script,
man page, spec, and tests together for a helper release. After merging, select
`package=github-copilot-installer` and `operation=build` in the manual COPR
workflow. It uploads only this helper to `<owner>/github-copilot-installer`;
`operation=project` creates/verifies that project without building. Verify
its signing key before Nimbus integration. Nothing is published by local
checks. See the [publishing workflow](../../README.md#workflow).
