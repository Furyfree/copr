%global debug_package %{nil}
%global app_version pin

Name:           obsidian-installer
Version:        0.1.0
Release:        0.1.app%{app_version}%{?dist}
Summary:        Automatically install the selected official Obsidian app
License:        MIT AND BSD-3-Clause
URL:            https://github.com/Furyfree/copr
Source0:        obsidian-installer-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
BuildRequires:  systemd-rpm-macros
Requires:       systemd
Requires:       util-linux >= 2.40
%{?systemd_requires}
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
Installing or upgrading this package automatically schedules installation of
Obsidian %{app_version} directly from its official GitHub release. A one-shot
systemd job verifies the package-selected SHA-256, extracts the AppImage and
creates desktop integration. Application bytes are not included in this RPM.
The job finishes separately from the DNF transaction; failures remain visible
in the service journal. Removing the package also removes the verified app
and desktop integration, while preserving settings and vaults.

%prep
%autosetup -n obsidian-installer-%{version}

%build
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go build -mod=vendor -trimpath -buildvcs=false -o obsidian-installer ./cmd/obsidian-installer

%check
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go test -mod=vendor ./...
./obsidian-installer --version

%install
install -Dpm 0755 obsidian-installer %{buildroot}%{_bindir}/obsidian-installer
install -Dpm 0644 obsidian-installer.service %{buildroot}%{_unitdir}/obsidian-installer.service
install -Dpm 0644 obsidian-installer.1 %{buildroot}%{_mandir}/man1/obsidian-installer.1
mkdir -p bundled-licenses
if test -d vendor; then
    find vendor -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp --parents '{}' bundled-licenses/ \;
fi
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%posttrans
if test -d /run/systemd/system; then
    systemctl daemon-reload || exit 1
    # A freshly installed unit may not be loaded yet and has no failure to reset.
    systemctl reset-failed obsidian-installer.service 2>/dev/null || :
    systemctl --no-block restart obsidian-installer.service || exit 1
    echo 'Obsidian %{app_version} installation queued after DNF. Check: systemctl status obsidian-installer.service'
else
    echo 'Obsidian installation needs a running systemd host. Run dnf reinstall obsidian-installer after boot.' >&2
fi

%preun
if [ "$1" -eq 0 ]; then
    # Stop running downloads and pending retries before removing their payload.
    if test -d /run/systemd/system; then
        systemctl stop obsidian-installer.service || exit 1
    fi
    # Keep the helper installed if ownership verification or cleanup fails.
    %{_bindir}/obsidian-installer uninstall --assumeyes || exit 1
fi
%systemd_preun obsidian-installer.service

%postun
%systemd_postun obsidian-installer.service

%files
%license LICENSE bundled-licenses
%doc README.md
%{_bindir}/obsidian-installer
%{_unitdir}/obsidian-installer.service
%{_mandir}/man1/obsidian-installer.1*
