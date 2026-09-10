%global debug_package %{nil}

Name:           wowup-cf-installer
Version:        0.1.0
Release:        0.1%{?dist}
Summary:        Installer helper for the official WoWUp CurseForge app
License:        MIT AND BSD-3-Clause
URL:            https://github.com/Furyfree/copr
Source0:        wowup-cf-installer-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
Requires:       squashfs-tools
Requires:       bash
Requires:       coreutils
Requires:       gtk2
Requires:       gtk3
Requires:       dbus-glib
Requires:       libdbusmenu
Requires:       libdbusmenu-gtk2
Requires:       nss
Requires:       alsa-lib
Requires:       mesa-libgbm
Requires:       libXScrnSaver
Requires:       libXtst

%description
An open-source helper for explicit installation, updates and removal of the
official WoWUp CurseForge AppImage. It verifies the official release metadata,
SHA-256 and artifact identity, extracts without executing the downloaded
runtime, and maintains an owned application bundle and desktop integration.
The application is not included. Installing or upgrading this helper does not
download the application or enable background updates. Removing only the
helper leaves the application installed.

%prep
%autosetup -n wowup-cf-installer-%{version}

%build
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go build -mod=vendor -trimpath -buildvcs=false -o wowup-cf-installer ./cmd/wowup-cf-installer

%check
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go test -mod=vendor ./...
./wowup-cf-installer --version

%install
install -Dpm 0755 wowup-cf-installer %{buildroot}%{_bindir}/wowup-cf-installer
install -Dpm 0644 wowup-cf-installer.1 %{buildroot}%{_mandir}/man1/wowup-cf-installer.1
mkdir -p bundled-licenses
if test -d vendor; then
    find vendor -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp --parents '{}' bundled-licenses/ \;
fi
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%files
%license LICENSE bundled-licenses
%doc README.md
%{_bindir}/wowup-cf-installer
%{_mandir}/man1/wowup-cf-installer.1*

%changelog
* Thu Sep 10 2026 COPR maintainers - 0.1.0-0.1
- Build the Go helper and its offline lifecycle tests from a dedicated source bundle.
- Maintain the MIT helper independently of Nimbus, with offline lifecycle tests.
