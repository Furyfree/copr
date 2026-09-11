# Source0 contains the reviewed release source and its vendored modules,
# rooted at nimbus-%{version}/.
%global debug_package %{nil}
%global source_sha256 a8cdf5b06bf59742d6bdaec529d625105f806299de91d5f45cbd5e8ae8c9b720

Name:           nimbus
Version:        0.4.0
Release:        0.1%{?dist}
Summary:        Personal Fedora workstation installer and system manager
License:        MIT AND BSD-3-Clause AND Apache-2.0 AND Unicode-DFS-2016
URL:            https://github.com/Furyfree/nimbus
Source0:        %{url}/releases/download/v%{version}/nimbus-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
BuildRequires:  git-core
BuildRequires:  gnupg2
BuildRequires:  python3
Requires:       dnf5
Requires:       rpm
Requires:       flatpak
Requires:       systemd
Requires:       git-core
Requires:       gnupg2
Requires:       sudo

%description
Nimbus coordinates native Fedora tools to inspect and apply a workstation's
selected definitions. The engine reads a separately selected checkout.
Installing this RPM does not initialize or provision the workstation.

%prep
printf '%s  %s\n' '%{source_sha256}' '%{SOURCE0}' | sha256sum --check --strict
%autosetup -n nimbus-%{version}

%build
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go build -mod=vendor -trimpath -buildvcs=false \
  -ldflags '-X github.com/Furyfree/nimbus/internal/version.Engine=%{version}' \
  -o nimbus ./cmd/nimbus

%check
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go test -mod=vendor ./...
./nimbus version
./nimbus validate --checkout .

%install
install -Dpm 0755 nimbus %{buildroot}%{_bindir}/nimbus
# Retain the bundled modules' notices without installing their source trees.
mkdir -p bundled-licenses
find vendor -type f \( -iname 'license*' -o -iname 'copying*' \
  -o -iname 'notice*' -o -iname 'copyright*' \) \
  -exec cp --parents '{}' bundled-licenses/ \;
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%files
%license LICENSE bundled-licenses licenses/Unicode-DFS-2016.txt
%{_bindir}/nimbus

%changelog
* Fri Sep 11 2026 Nimbus maintainers - 0.4.0-0.1
- Update clean Nimbus and Chezmoi repositories during sync.
- Upgrade through Topgrade before restarting the updated engine for sync.
- Add upstream Zed installation and shared browser/private-mode fallbacks.
- Correct VM health checks and conditional session notices.

* Thu Sep 10 2026 Nimbus maintainers - 0.3.1-0.1
- Reuse empty pre-mounted Snapper storage with reviewed, retryable setup.
- Ask for the machine on first installation and restore the Fedora guide.

* Thu Sep 10 2026 Nimbus maintainers - 0.3.0-0.1
- Separate sync from Topgrade upgrades and simplify workstation ownership.
- Add bounded Snapper snapshots and package version constraints.
- Finalize interrupted changes and report Snapper configuration drift.

* Wed Sep 09 2026 Nimbus maintainers - 0.2.3-0.1
- Fix requested package resolution in DNF dependency installation sections.

* Wed Sep 09 2026 Nimbus maintainers - 0.2.2-0.1
- Package the Go modernization and command, planning, and ownership fixes.
- Write state marker schema 3; keep schemas 1 and 2 readable.

* Wed Sep 09 2026 Nimbus maintainers - 0.2.1-0.1
- Package promptless init, toolkit dependencies, and Ghostty recovery fixes.
- Retain the tested plain Hyprland session; defer UWSM adoption.

* Tue Sep 08 2026 Nimbus maintainers - 0.2.0-0.1
- Package system resources, Noctalia login, recovery session, and installer fixes.

* Mon Sep 07 2026 Nimbus maintainers - 0.1.1-0.1
- Package the installation prompt, logging, and repository-convergence fixes.

* Mon Sep 07 2026 Nimbus maintainers - 0.1.0-0.1
- Prepare the first engine RPM; distribution gates remain open.
