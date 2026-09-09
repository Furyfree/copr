# Nimbus RPM packaging

RPM packaging for the Nimbus engine. The engine and definitions remain
in `Furyfree/nimbus`; `Furyfree/copr` holds distribution packaging. This recipe
supersedes the initial local `nimbus-rpm` draft. The hosted project is
[`furyfree/nimbus`](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/).

## COPR project

- Project name: `nimbus`, under the owner's authenticated Fedora account.
- Target: `fedora-44-x86_64` only.
- Description: Personal Fedora workstation installer and system manager.
- Build submission: an explicitly dispatched Actions run uploads a reviewed
  source RPM, with no automatic rebuild webhook.
- Install payload: `/usr/bin/nimbus` and license notices. No scriptlets,
  selector, receipts, checkout, services, or workstation provisioning.
- Updates and removal: native DNF operations.

## Build evidence and remaining gate

Nimbus requires Go 1.26.7, supplied by Fedora 44's native repositories.
Its existing dependencies require at most Go 1.25, and the complete Nimbus
local gate passes with Go 1.26.7. No alternate compiler repository or prebuilt
engine artifact is needed for this recipe.

[Release v0.1.0](https://github.com/Furyfree/nimbus/releases/tag/v0.1.0) contains
the reviewed vendored source from Nimbus commit
`e1c081bb31ddc8d9c3823a8859b43c6188fe50d3`. Its archive SHA-256 is
`54e113b54c11d09def622c639a59e4ad7d30378020e3bd8d77c57c4072950808`.

[COPR build 10958015](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10958015/)
succeeded with build networking disabled. The hosted SRPM contains the exact
published source archive. The signed RPM contains the engine and license
notices, with no scriptlets; its binary reports `nimbus 0.1.0` and validates
the definitions shipped with that release. Current definitions require
engine 0.2.0 or newer.

The official project key has fingerprint
`8FF8 E546 C3AB E441 46CF A411 DD1D 48E2 CA0E 9F6A`. Native RPM verification in
an isolated keyring containing only that key passed with signatures and
digests required. Nimbus ships the reviewed key for bootstrap. The restored
Fedora VM installation, upgrade, and removal drill remains a separate gate.

Configure `COPR_OWNER` and `COPR_CONFIG` as described in the
[repository setup](../../README.md#account-setup) when setting up another
account. Never commit credentials or private keys. Do not upload working-tree
snapshots; release source comes from the approved tag after its complete checks.

## Version 0.2.0 release

The 0.2.0 recipe selected `v0.2.0` for system resource ownership, Noctalia login,
recovery-session definitions, and installer improvements. Before publication,
the candidate passed installation, reboot, and normal Hyprland login on Fedora
44.
The keyring PAM dependency is included in the versioned desktop definitions;
it is not an unconditional engine RPM dependency.

[Release v0.2.0](https://github.com/Furyfree/nimbus/releases/tag/v0.2.0) selects
Nimbus commit `12781bcc2673123b0f97dd7b4f9ae1216d648815`. Its source archive
SHA-256 is
`b52ecbab8e3b7d197ba449f986c062cf7c3fcf4c4e801be33d46a3037ed5adc2`.

[COPR build 10961922](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10961922/)
succeeded for `nimbus-0.2.0-0.1.fc44.x86_64` with build networking disabled.
The uploaded SRPM contains the byte-identical published source archive.
Verification in an isolated RPM database containing only the pinned project
key passed the signatures and digests. Payload inspection confirms only the
engine and license notices, with no scriptlets. The extracted binary reports
`nimbus 0.2.0` and validates all three definitions as an unprivileged user in a
network-disabled container.

The manual release and COPR workflows both passed. A signed-package VM trial,
keyring unlocking, recovery and portal drills remain tracked in Nimbus's tasks;
successful packaging does not establish those behaviors.

## Version 0.2.1 release

The recipe selects [v0.2.1](https://github.com/Furyfree/nimbus/releases/tag/v0.2.1)
from Nimbus commit `77ade6a5a1cc03fa4e358adfbd9934e6c2bd644b`. It packages
promptless init, the missing toolkit dependencies, and a Ghostty recovery
session that permits removal of Foot, Kitty and nwg-panel. The owner accepted
the VM-tested plain Hyprland configuration; UWSM adoption is deferred for
research. Portal and recovery drills remain recorded follow-ups.

The published source archive SHA-256 is
`5a32013f2149c01facdad2ea5a1eb0d4290d3273792f4e4cce63286d44fbe100`.
Its tagged files are byte-identical to the approved commit; dependencies and
license notices are unchanged from 0.2.0. The build must use this archive. Packaging
checks do not establish the deferred desktop behaviors or replace a VM test of
the final signed package.

## Version 0.2.2 release

The recipe selects [v0.2.2](https://github.com/Furyfree/nimbus/releases/tag/v0.2.2)
from Nimbus commit `406f47a1e90a16b8be956588c60199541a5d45c8`. It packages the
Go 1.26.7 modernization and fixes to command output, native inspection,
transaction approval, and resource ownership. Verified file receipts survive
temporary-payload cleanup failures, which remain visible in the final report.

The approved source archive SHA-256 is
`a739540ef05b89a136e6e9e2bd2d7c58dd6480de46e744e88d04cade64b491ff`.
The recipe verifies this pin before extracting Source0 or compiling it.
Update the version and checksum together after verifying each release.

State writes now use marker schema 3; schemas 1 and 2 remain readable.
Receipt and baseline contents remain schema 2. Older engines refuse the new
marker, so downgrading the binary alone is not a state rollback. The deferred
application/Topgrade work is roadmap-only. Native VM lifecycle, portal, and
recovery drills remain open; successful packaging does not establish them.

## Source and validation

[`nimbus.spec`](nimbus.spec) expects `nimbus-0.2.2-vendor.tar.gz`, containing the
full reviewed Nimbus source rooted at `nimbus-0.2.2/`, with modules generated by
`go mod vendor`, published as an asset of the matching upstream release.
The upstream `just tag VERSION` and `just release VERSION` commands prepare
a draft release from a checked commit on `main`. Publish the reviewed draft
before dispatching this repository's `build` operation. Preserve LICENSE,
all fixtures and definitions needed by the tests, and dependency license files;
exclude `.git`, local runtime state, and unpublished working-tree changes.
Record the approved commit and archive SHA-256 with the release artifacts.

Compilation and tests use vendored modules with network/toolchain downloading
disabled. System Python 3 is a build requirement for the isolated handoff
regressions. `%check` runs all Go tests and validates the packaged definitions
fixture; those definitions are not installed by the RPM. Review the bundled
license expression against the final source archive before publishing. The
RPM retains the Unicode-DFS-2016 notice for uniseg's Unicode 15.0.0 tables
in addition to vendored module notices and the Go license.

To reproduce source preparation locally, build an SRPM
with `python3 scripts/srpm.py --spec packages/nimbus/nimbus.spec --outdir /tmp/nimbus-srpm`
from the repository root, then build it in an isolated Fedora 44 buildroot.
Inspect its payload, dependencies,
scriptlets, and engine version.
Test install, upgrade, and removal in a disposable VM before declaring the
installation flow validated.

For later signing-key changes, verify the new public key and its signed RPM
before updating Nimbus's bootstrap pin. Repeat the one-liner drill from a
clean Fedora VM. Never use a placeholder pin or `--nogpgcheck`.

Reference: [COPR user documentation](https://docs.copr.fedorainfracloud.org/user_documentation.html).
