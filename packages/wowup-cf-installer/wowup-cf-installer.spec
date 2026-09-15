%global debug_package %{nil}
%global app_version 2.23.1

Name:           wowup-cf-installer
Version:        0.3.0
Release:        0.1.app%{app_version}%{?dist}
Summary:        Automatically install the selected official WoWUp CurseForge app
License:        MIT AND BSD-3-Clause
URL:            https://github.com/Furyfree/copr
Source0:        wowup-cf-installer-%{version}-vendor.tar.gz
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
WoWUp-CF %{app_version} directly from its official GitHub release. A one-shot
systemd job verifies the package-selected SHA-256, extracts the AppImage and
creates desktop integration. Application bytes are not included in this RPM.
The job finishes separately from the DNF transaction; failures remain visible
in the service journal. Removing the package also removes the verified app
and desktop integration, while preserving settings, games and addons.

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
install -Dpm 0644 wowup-cf-installer.service %{buildroot}%{_unitdir}/wowup-cf-installer.service
install -Dpm 0644 wowup-cf-installer.1 %{buildroot}%{_mandir}/man1/wowup-cf-installer.1
mkdir -p bundled-licenses
if test -d vendor; then
    find vendor -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp --parents '{}' bundled-licenses/ \;
fi
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%posttrans
if test -d /run/systemd/system; then
    systemctl daemon-reload || exit 1
    # A freshly installed unit may not be loaded yet and has no failure to reset.
    systemctl reset-failed wowup-cf-installer.service 2>/dev/null || :
    systemctl --no-block restart wowup-cf-installer.service || exit 1
    echo 'WoWUp-CF %{app_version} installation queued after DNF. Check: systemctl status wowup-cf-installer.service'
else
    echo 'WoWUp-CF installation needs a running systemd host. Run dnf reinstall wowup-cf-installer after boot.' >&2
fi

%preun
if [ "$1" -eq 0 ]; then
    # Stop running downloads and pending retries before removing their payload.
    if test -d /run/systemd/system; then
        systemctl stop wowup-cf-installer.service || exit 1
    fi
    # Keep the helper installed if ownership verification or cleanup fails.
    %{_bindir}/wowup-cf-installer uninstall --assumeyes || exit 1
fi
%systemd_preun wowup-cf-installer.service

%postun
%systemd_postun wowup-cf-installer.service

%files
%license LICENSE bundled-licenses
%doc README.md
%{_bindir}/wowup-cf-installer
%{_unitdir}/wowup-cf-installer.service
%{_mandir}/man1/wowup-cf-installer.1*

%changelog
* Tue Sep 15 2026 COPR maintainers - 0.3.0-0.1.app2.23.1
- Add explicit unprivileged purge of the current user's WoWUp profile.

* Tue Sep 15 2026 COPR maintainers - 0.2.0-0.1.app2.23.1
- Automatically queue verified app installation on package install and upgrade.
- Pin the selected app version and digest; preserve lifecycle ownership checks.
- Remove verified application files on final package removal, preserving user data.

* Thu Sep 10 2026 COPR maintainers - 0.1.0-0.1
- Build the Go helper and its offline lifecycle tests from a dedicated source bundle.
- Maintain the MIT helper independently of Nimbus, with offline lifecycle tests.
