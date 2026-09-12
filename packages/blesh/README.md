# ble.sh RPM packaging

`blesh` provides Bash syntax highlighting, history suggestions, vi editing and
completion. The noarch RPM includes contrib modules, including
`integration/fzf-menu`, and targets the `fedora-44-x86_64` COPR chroot.

## Sources and ownership

The recipe selects upstream's 2026-09-08 development snapshot:

- [ble.sh source](https://github.com/akinomyoga/ble.sh/tree/d81fd54feb0d996fdff20dca27eaf0201f7015cc):
  `d81fd54feb0d996fdff20dca27eaf0201f7015cc`.
- [Matching contrib submodule](https://github.com/akinomyoga/blesh-contrib/tree/d2109203480a7dfe1dead5f5f8f9f15a9146c90d):
  `d2109203480a7dfe1dead5f5f8f9f15a9146c90d`.

Both archive SHA-256 values are pinned in `blesh.spec` and verified before
extraction. Update the commits, digests and RPM release together. The core
archive exports Git version metadata in `make/.git-archive-export.mk`, so
neither a `.git` directory nor a generated tarball is needed. Only source
preparation downloads anything; binary RPM builds and upstream tests run
offline.

The upstream installer places runtime files in `/usr/share/blesh`, documentation
in `/usr/share/doc/blesh`, and licenses in `/usr/share/licenses/blesh`. The core
and contrib licenses are BSD-3-Clause; imported vim-airline themes carry MIT
notices. `vim-airline-themes-LICENSE` is copied from the
[upstream license](https://github.com/vim-airline/vim-airline-themes/blob/77aab8c6cf7179ddb8a05741da7e358a86b2c3ab/LICENSE).
Individual theme copyright notices remain in the installed Bash files.

There are no RPM scriptlets or shell startup changes. Chezmoi owns shell
activation and fzf configuration. Runtime files are read-only to ordinary users;
cache and runtime state use ble.sh's per-user XDG locations. The package omits
upstream's shared writable cache and runtime directories. `ble-update` prints
the DNF upgrade command and returns failure without downloading or modifying
the installation. Updates and removal use DNF.

## Build and publication

From a Fedora environment with this repository's build tools:

```sh
just prepare blesh /tmp/blesh-srpm
just check-package blesh
rpmbuild --rebuild /tmp/blesh-srpm/blesh-*.src.rpm
```

Run the binary rebuild in a disposable environment with networking disabled.
The shared tooling image includes this recipe's build dependencies.

After merging, explicit publication from remote `main` uses:

```sh
just publish blesh
```

That command creates/verifies the owner's `blesh` project and submits one SRPM.
Adding the recipe or opening a pull request does not publish it. Nimbus should
select the package only after publication and verification of the COPR key.

Once published, installation is:

```sh
sudo dnf copr enable furyfree/blesh
sudo dnf install blesh
```

Existing dotfiles already look for `/usr/share/blesh/ble.sh`. For a separate
configuration, follow upstream's
[Bash setup and fzf integration instructions](https://github.com/akinomyoga/ble.sh#13-set-up-bashrc).

## Verification

The initial recipe passed the complete `just check` gate in the shared Fedora
44 container with networking disabled. A real SRPM rebuilt offline, including
7,842 upstream assertions with zero failures or crashes and 119 upstream skips.
The installed update hook returned failure with the DNF instruction as intended.

Inspection of the 345 RPM paths found only runtime files, documentation and
licenses, with no writable shared directories, scriptlets or broken symlinks.
Native RPM installation, verification and removal passed in a disposable
container. An isolated Bash session using the extracted RPM and the existing
dotfiles fzf configuration exercised ordinary Tab completion with the real fzf
picker, history suggestions and syntax colors. Hosted COPR publication and
verification of the signed package remain separate steps.
