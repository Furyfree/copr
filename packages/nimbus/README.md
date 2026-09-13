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
engine 0.5.0 or newer.

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

## Version 0.2.3 release

The recipe selects [v0.2.3](https://github.com/Furyfree/nimbus/releases/tag/v0.2.3)
from Nimbus commit `9999a071a4a3f37cd1cd3453f130fa3bff380922`. It fixes fresh
installation stopping on `gcc-c++` when DNF lists an explicitly requested
package as a dependency. Matching includes all installation sections while
preserving exact provider evidence, repository reporting, and native receipts.

The approved source archive SHA-256 is
`69237bd5c7f52bfc7c9d02766b6a0cadc547fcfa98958530cffe766cc8682f8c`.
All 199 tagged files and executable bits match the approved commit. Vendored
modules and license notices are unchanged from 0.2.2. The complete Nimbus gate
and offline vendored build/tests pass with Go 1.26.7. On the affected Fedora 44
VM, the candidate produces a complete plan resolving all 118 package requests;
released 0.2.2 still blocks. Installation retry and desktop/recovery drills
remain pending.

## Version 0.3.0 release

The recipe selects [v0.3.0](https://github.com/Furyfree/nimbus/releases/tag/v0.3.0)
from Nimbus commit `a8388deb4b5af5e63cb255ca543fd46c8c608a9f`. It separates
system sync from Topgrade upgrades, adds bounded native Snapper snapshots and
package version constraints, and simplifies command and ownership boundaries.
Interrupts stop later work and finalize snapshots after the native command
returns. Status reports Snapper configuration drift.

The approved source archive SHA-256 is
`30efb121dcd82f28fdfd2177e422578839fe8486aa062e112161474e3ae487d5`.
All 246 tagged files and executable bits match the approved commit. Module
versions and license notices are unchanged from 0.2.3. The complete Nimbus
gate, focused race tests, PR CI, and offline vendored build/tests passed.

Update the engine RPM before updating an existing definition checkout or
applying the matching Chezmoi Topgrade configuration: current definitions
require engine 0.3.0. The installer validates an existing engine but does not
upgrade it automatically. Installation, upgrade, and native Snapper behavior
still need testing with the signed package in the VM.

## Version 0.3.1 release

The recipe selects [v0.3.1](https://github.com/Furyfree/nimbus/releases/tag/v0.3.1)
from Nimbus commit `9d00ecb1f12108b2d2916997f330392a73ae2437`. It fixes initial
setup with an existing empty snapshot mount and adds the first-install machine
prompt. Storage adoption preserves mounts and supports interrupted retries.

The source archive SHA-256 is
`f56fa5320fa05abd6663ae4cc9e06b55b95b39876853fe02aef1346d2d0b40d8`.
All 249 tagged files and executable bits match the commit. Dependencies and
license notices are unchanged from 0.3.0. Nimbus local checks, definition
validation and the offline vendored release build/tests passed. Native
Btrfs/SELinux behavior and the installation retry still need a VM trial.

## Version 0.4.0 release

The recipe selects [v0.4.0](https://github.com/Furyfree/nimbus/releases/tag/v0.4.0)
from Nimbus commit `dde39045b9276281fd5338aac885a27ce5a96324`. Sync updates clean
Nimbus and Chezmoi repositories before reconciliation. Combined upgrades run
Topgrade first and restart the updated engine for sync. This release also adds
upstream Zed installation, shared browser/private-mode fallbacks and reporting
fixes. UKI generation and TPM auto-unlock remain planned work.

The source archive SHA-256 is
`a8cdf5b06bf59742d6bdaec529d625105f806299de91d5f45cbd5e8ae8c9b720`.
All 254 tagged files and executable bits match the commit. Vendored dependencies
and license notices are unchanged from 0.3.1. Nimbus `just check`, definition
validation and the GitHub vendored release build/tests passed.

[Build 10976629](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10976629/)
published `nimbus-0.4.0-0.1.fc44.x86_64`. The local offline container gate and
publication workflow passed. Both prepared and published SRPMs retain the exact
archive. RPM signatures and digests passed against the pinned project key.
The payload contains only the engine and license notices, with no scriptlets;
the extracted engine reports 0.4.0 and validates all three machines in a
network-disabled Fedora container. Desktop installation remains untested.

## Version 0.4.1 release

The recipe selects [v0.4.1](https://github.com/Furyfree/nimbus/releases/tag/v0.4.1)
from commit `e067e7f9300968322f948bf2e6ed1be932f75d93`. It adds machine login-shell
selection, explicit Tailscale operator and Proton-CachyOS actions, shared power
management and the desktop PDF backend. The prepared XDG system defaults file
remains unselected until ownership migration is supported.

The source archive SHA-256 is
`ff1bb6ba7935432dad5b779e0a69b347dc8ec869eefdfe5a7546030c732db0c6`.
The archive matches the tagged source and executable bits. Dependencies and
license notices are unchanged from 0.4.0. Nimbus checks, machine validation and
the GitHub offline vendored build/tests passed. COPR build completion and
signed-package installation checks remain pending.

## Version 0.4.2 release

The recipe selects [v0.4.2](https://github.com/Furyfree/nimbus/releases/tag/v0.4.2)
from commit `6d523c54957d5e277c00d74665865e2e0d9d36d2`. Combined sync prepares
package sources, system setup and Chezmoi before Topgrade, then starts the
updated engine for a final sync. Failed or declined setup prevents upgrades.
Definitions select ble.sh and the Noctalia LibrePods fork from verified COPRs.

The source archive SHA-256 is
`339dbd35dfbe50f6275448219e420549bec15ef37261515c887a012e896b9006`.
All 264 tagged files and executable bits match the commit. Module inputs and
license notices are unchanged from 0.4.1. Nimbus checks, machine validation and
the GitHub offline vendored build/tests passed. Signed-package installation
remains untested. The old engine requires a direct DNF upgrade before using
`nimbus sync --upgrade` with these definitions.

## Version 0.4.3 release

The recipe selects [v0.4.3](https://github.com/Furyfree/nimbus/releases/tag/v0.4.3)
from commit `e7690ff10073296cad24fe6a6e58a6a63989fe1e`. The development profile
adds Zeron and its browser dependencies. Installer previews disclose its user
service and lingering effects. Definitions select Zsh as the default login
shell and disable the settings-loader autostart on NVIDIA machines.

The source archive SHA-256 is
`6716cd80d7092a2b7f84a50f71de02b64a7c71dfea45f24e64dec2c4e8d3fd4d`.
All 269 tagged files and executable bits match the commit. Module inputs and
23 vendored dependency notices are unchanged from 0.4.2. Nimbus's local gate,
CI and offline vendored release build/tests passed. The new installer field
requires engine 0.4.3; upgrade the Nimbus RPM before syncing these definitions.
The complete packaging gate passed in an isolated Fedora 44 container. The
prepared source RPM retains the exact published archive and selected spec.
Fresh installation and hardware behavior still need an installation trial.

## Version 0.4.4 release

The recipe selects [v0.4.4](https://github.com/Furyfree/nimbus/releases/tag/v0.4.4)
from commit `c8c0f6137e841eaca718150ff7be5af196e77074`. It adds
`nimbus postinstall noctalia-plugins` to inspect enabled plugin runtime exports,
update affected sources through Noctalia and wait for readable manifests and
entry scripts. Plugin selection and runtime state remain with Chezmoi/Noctalia.

The source archive SHA-256 is
`06c3fed54492fa9a41a1985e10ab96a4e1d547d182b86b3180ed8c9758f5176c`.
All 271 tagged files and executable bits match the commit. Dependencies and
23 vendored notices are unchanged from 0.4.3. Nimbus checks, definition
validation and the GitHub offline vendored build/tests passed. The full offline
packaging gate passed in an isolated Fedora 44 container. The prepared source
RPM preserves the exact archive and spec.

[Build 10980508](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10980508/)
published `nimbus-0.4.4-0.1.fc44.x86_64`; full and selected-package CI and the
publication workflow passed. The published source RPM preserves the exact
archive and spec. Native RPM signature and digest checks passed against an
isolated keyring containing only Nimbus's pinned project key. The binary RPM
SHA-256 is
`b396c202e3fec01348b1f3898954bbb832e72bbc32a6054e13b05654333e6022`.

The payload contains only the engine and license notices, without scriptlets
or triggers. In an unprivileged, network-disabled Fedora container, the
extracted engine reports 0.4.4, validates all three tagged machines and
recognizes the laptop's Noctalia task in preview mode. Missing Noctalia is
reported as blocked. No workstation upgrade or plugin repair was performed;
an installed-session trial remains pending.

## Version 0.4.5 release

The recipe selects [v0.4.5](https://github.com/Furyfree/nimbus/releases/tag/v0.4.5)
from commit `c324a94bb0d1fcf5a65d23f07301f436f7a94716`. It fixes premature
verification failure while Noctalia updates plugins in the background. Nimbus
retries verification for up to two minutes without repeating source updates;
persistent failures retain the latest diagnostic.

The source archive SHA-256 is
`e6d09a8466f230f16bafbb46c71e6076cd2ebe2371c80aaebaeb754cd6957d3b`.
All 271 tagged files and executable bits match the commit. Dependencies and
23 vendored notices are unchanged from 0.4.4. Nimbus checks, definition
validation and the GitHub offline vendored build/tests passed. The full offline
packaging gate passed in an isolated Fedora 44 container.

[Build 10980526](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10980526/)
published `nimbus-0.4.5-0.1.fc44.x86_64`. Full and selected-package CI and the
publication workflow passed. Prepared and published source RPMs preserve the
exact archive and spec. Native RPM signatures and digests passed with an
isolated database containing only Nimbus's pinned key. Binary RPM SHA-256 is
`6fe32b08a05168e3e68995e388f246d65c16a042235039559616b065f5ac97e8`.

The payload contains only the engine and licenses, without scriptlets or
triggers. The extracted engine reports 0.4.5, validates all three tagged
machines and recognizes the laptop's Noctalia task in an unprivileged,
network-disabled Fedora container. No workstation upgrade was performed;
a fresh installed-session repair trial remains pending.

## Version 0.4.6 release

The recipe selects [v0.4.6](https://github.com/Furyfree/nimbus/releases/tag/v0.4.6)
from commit `9e529807149cec86123bfe07ec97aa88f69df00f`. It adds verified native
Hyprland plugin setup, AccountsService picture registration and the minimal
Noctalia greeter configuration. Completed post-install results are concise.

The source archive SHA-256 is
`2d891a105e85b2ffb8ff291ede5e38163f05531144f504dccdeaeb141c845a95`.
All 279 tagged files and executable bits match the commit. Dependencies and
23 vendored notices are unchanged from 0.4.5. Nimbus local checks, definition
validation, CI and the GitHub offline vendored build/tests passed.

[COPR build 10980604](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10980604/)
published `nimbus-0.4.6-0.1.fc44.x86_64`. The full offline Fedora packaging gate,
full and selected-package CI, and publication workflow passed. Prepared and
published source RPMs preserve the exact archive and reviewed spec. Native RPM
signatures and digests passed with only Nimbus's pinned key in an isolated
database. Binary RPM SHA-256 is
`e833600b0315f04e18dd4514517f3f339e68cf7a2c013e304062aa5510b4cf39`.

The payload contains only the engine and licenses, without scripts or triggers.
In an unprivileged, network-disabled Fedora container, the extracted engine
reports 0.4.6, validates all three tagged machines and recognizes both new
post-install tasks. Missing prerequisites correctly block their previews.
No workstation upgrade was performed. Fresh laptop plugin setup and the
promoted greeter mount still require installed-session testing.

## Version 0.5.0 release

The recipe selects [v0.5.0](https://github.com/Furyfree/nimbus/releases/tag/v0.5.0)
from commit `a3ec141c5578ebd26d61e61e458568157d0446b1`. It checks fresh engine RPM
metadata before Git updates, upgrades/restarts Nimbus first when requested,
syncs once and then runs Topgrade. Guided postinstall commands and Nimbus-owned
setup notes use private per-machine evidence; logged native progress remains
live, with one combined final report. SSH integration remains an explicit opt-in.

The source archive SHA-256 is
`2722499273598eb23f5f1bef2768d295c05254400750a1eb52ea1236ca63ca6b`.
All 296 tagged regular files and executable modes match the release commit.
All 754 vendored files and 23 dependency notices are unchanged from 0.4.6.
Nimbus's full gate, definition validation, CI and offline vendored release
build/tests pass. Chezmoi's 147 tests pass with three optional skips.

A disposable signed-RPM drill verifies initial installation, stale-cache
refresh, update refusal, engine replacement/restart and failure reporting.
Full graphical init/login, real-account setup, hardware and snapshot recovery
remain open. The installed 0.4.6 engine needs one direct DNF upgrade before the
new workflow can manage engine updates. Current definitions require 0.5.0.

The complete offline Fedora packaging gate and a source-RPM rebuild pass.
The rebuild uses a normal unprivileged account; an initial anonymous container
UID failed current-user fixture checks. The successful retry changed no source
or tests. Prepared and COPR-published source RPMs contain the exact archive and
reviewed spec. All three repositories' GitHub checks passed.

[COPR build 10981938](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10981938/)
is submitted through the
[publication workflow](https://github.com/Furyfree/copr/actions/runs/34770315892).
At handoff it is waiting for Copr DistGit source import alongside dozens of
other builds. The workflow continues automatically. Signed binary publication,
signature/payload verification and extracted-binary smoke tests remain pending.
No desktop or laptop package update was run.

## Version 0.5.1 release

The recipe selects [v0.5.1](https://github.com/Furyfree/nimbus/releases/tag/v0.5.1)
from commit `95bb78202344bc90c4e6c4372ffebb552fd9c82b`. It corrects MOK
verification, verifies existing guided setup, offers native 1Password sign-in
recovery and skips empty system update transactions. Terminal colors, wrapping,
Doctor results and maintenance previews are clearer. Definitions remain
compatible with engine 0.5.0.

The source archive SHA-256 is
`928927f49feda901d039c59ac595047dc09eb931345cf845d8a6f2fb68462988`.
All 310 tagged files and executable modes match the release commit; all 754
vendored files and 23 dependency notices are unchanged from 0.5.0. Nimbus's
local gate, CI, race tests and Go 1.26.7 offline vendored release build/tests
pass. The full offline packaging gate and source RPM rebuild also passed.

[Build 10982208](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10982208/)
published `nimbus-0.5.1-0.1.fc44.x86_64`. Full and selected-package CI and the
publication workflow passed. Prepared and published source RPMs preserve the
exact archive and spec. Published repository metadata and its package checksum
match. Native RPM signatures and digests passed against the pinned project key
in an isolated database. Binary RPM SHA-256 is
`a88f3a2aaddb0b8641dd3c923d7fce0252f5c2770277e7a08a26402934febcab`.

The payload contains only the engine and licenses, without scriptlets or
triggers. In an unprivileged, offline Fedora container, the extracted engine
reports 0.5.1, validates all three tagged machines and shows postinstall help.
No workstation upgrade or authentication was performed; installed-session
verification remains with the owner.

## Version 0.5.2 release

The recipe selects [v0.5.2](https://github.com/Furyfree/nimbus/releases/tag/v0.5.2)
from commit `94f448a560dd5e556eb6bb651265a2e29b6eda69`. It adds guided
agent-proxy setup and owned model refresh after successful combined upgrades,
explicit Noctalia lockscreen-widget repair, and consistent prompt/result
styling. Definitions remain compatible with engine 0.5.0.

The source archive SHA-256 is
`d6f99a5b62e35f9b754b0b5b2475456e517ba437a09cc66170289f55f62fbc36`.
All 332 tagged files and executable modes match the archive; 754 vendored
files and 23 dependency notices are unchanged from 0.5.1. The RPM also retains
the embedded upstream patch's MIT notice. Node is a build dependency so model
adapter tests run during the offline package build; runtime Node remains a
Mise-owned prerequisite of the optional setup task.

Nimbus's local gate, definition validation, CI and offline vendored release
tests passed. Chezmoi's matching configuration gate passed. The complete
packaging gate and an unprivileged offline SRPM rebuild passed; the prepared
SRPM contains the exact published archive and reviewed spec. Antigravity
sign-in and a visual laptop lockscreen test remain outside the packaging checks.

[COPR build 10982500](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/build/10982500/)
published `nimbus-0.5.2-0.1.fc44.x86_64`; its
[publication workflow](https://github.com/Furyfree/copr/actions/runs/34786152917)
passed. The published SRPM preserves the exact archive and spec. All three
repositories' release CI checks passed. Repository metadata and the binary
checksum match:
`f61a2cec904b4ec70e8ae0dc19c5fd17365f739ff7f2c2ff3093dc2f8282d9c4`.

Signatures and digests passed in an isolated RPM database containing only the
pinned project key. The payload contains only Nimbus and licenses, without
scriptlets or triggers. An unprivileged offline container confirms version
0.5.2, all three machine definitions and help for both new tasks. No host
installation or laptop changes were performed during release verification.

## Source and validation

[`nimbus.spec`](nimbus.spec) expects `nimbus-0.5.2-vendor.tar.gz`, containing the
full reviewed Nimbus source rooted at `nimbus-0.5.2/`, with modules generated by
`go mod vendor`, published as an asset of the matching upstream release.
The upstream `just tag VERSION` and `just release VERSION` commands prepare
a draft release from a checked commit on `main`. Publish the reviewed draft
before dispatching this repository's `build` operation with `package=nimbus`.
Preserve LICENSE,
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
with `just prepare nimbus /tmp/nimbus-srpm`
from the repository root, then build it in an isolated Fedora 44 buildroot.
Inspect its payload, dependencies,
scriptlets, and engine version.
Test install, upgrade, and removal in a disposable VM before declaring the
installation flow validated.

For later signing-key changes, verify the new public key and its signed RPM
before updating Nimbus's bootstrap pin. Repeat the one-liner drill from a
clean Fedora VM. Never use a placeholder pin or `--nogpgcheck`.

Reference: [COPR user documentation](https://docs.copr.fedorainfracloud.org/user_documentation.html).
