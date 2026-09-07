# First-release packaging draft. Source0 must contain the reviewed source
# tree and its vendored modules, rooted at nimbus-0.1.0/.
%global debug_package %{nil}

Name:           nimbus
Version:        0.1.0
Release:        0.1%{?dist}
Summary:        Personal Fedora workstation installer and system manager
License:        MIT AND BSD-3-Clause AND Apache-2.0 AND Unicode-DFS-2016
URL:            https://github.com/Furyfree/nimbus
Source0:        %{url}/releases/download/v%{version}/nimbus-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
BuildRequires:  git-core
BuildRequires:  gnupg2
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
* Mon Sep 07 2026 Nimbus maintainers - 0.1.0-0.1
- Prepare the first engine RPM; distribution gates remain open.
