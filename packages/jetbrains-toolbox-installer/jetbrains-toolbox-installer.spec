%global debug_package %{nil}
%global app_version pin

Name:           jetbrains-toolbox-installer
Version:        0.1.0
Release:        0.1.app%{app_version}%{?dist}
Summary:        Automatically install the selected official JetBrains Toolbox App
License:        MIT AND BSD-3-Clause
URL:            https://github.com/Furyfree/copr
Source0:        jetbrains-toolbox-installer-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
BuildRequires:  systemd-rpm-macros
Requires:       systemd
Requires:       util-linux >= 2.40
%{?systemd_requires}
Requires:       bash
Requires:       coreutils
Requires:       xdg-utils
Requires:       alsa-lib
Requires:       fontconfig
Requires:       freetype
Requires:       glib2
Requires:       harfbuzz
Requires:       libglvnd-glx
Requires:       libpng
Requires:       libstdc++
Requires:       libxml2
Requires:       libX11
Requires:       libXext
Requires:       libXi
Requires:       libXrender
Requires:       libXtst
Requires:       libxcb

%description
Installing or upgrading this package automatically schedules installation of
JetBrains Toolbox %{app_version} directly from JetBrains. A one-shot systemd
job verifies the published SHA-256, unpacks the archive into a root-owned
bundle and creates the launcher and desktop entry. Application bytes are not
included in this RPM. The launcher records the autostart opt-out before the
first launch, so Toolbox does not start with the desktop session unless the
user enables that in its settings. The job finishes separately from the DNF
transaction; failures remain visible in the service journal. Removing the
package also removes the verified app and desktop integration, while
preserving installed IDEs, projects and settings.

%prep
%autosetup -n jetbrains-toolbox-installer-%{version}

%build
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go build -mod=vendor -trimpath -buildvcs=false -o jetbrains-toolbox-installer ./cmd/jetbrains-toolbox-installer

%check
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go test -mod=vendor ./...
./jetbrains-toolbox-installer --version

%install
install -Dpm 0755 jetbrains-toolbox-installer %{buildroot}%{_bindir}/jetbrains-toolbox-installer
install -Dpm 0644 jetbrains-toolbox-installer.service %{buildroot}%{_unitdir}/jetbrains-toolbox-installer.service
install -Dpm 0644 jetbrains-toolbox-installer.1 %{buildroot}%{_mandir}/man1/jetbrains-toolbox-installer.1
mkdir -p bundled-licenses
if test -d vendor; then
    find vendor -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp --parents '{}' bundled-licenses/ \;
fi
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%posttrans
if test -d /run/systemd/system; then
    systemctl daemon-reload || exit 1
    # A freshly installed unit may not be loaded yet and has no failure to reset.
    systemctl reset-failed jetbrains-toolbox-installer.service 2>/dev/null || :
    systemctl --no-block restart jetbrains-toolbox-installer.service || exit 1
    echo 'JetBrains Toolbox %{app_version} installation queued after DNF. Check: systemctl status jetbrains-toolbox-installer.service'
else
    echo 'JetBrains Toolbox installation needs a running systemd host. Run dnf reinstall jetbrains-toolbox-installer after boot.' >&2
fi

%preun
if [ "$1" -eq 0 ]; then
    # Stop running downloads and pending retries before removing their payload.
    if test -d /run/systemd/system; then
        systemctl stop jetbrains-toolbox-installer.service || exit 1
    fi
    # Keep the helper installed if ownership verification or cleanup fails.
    %{_bindir}/jetbrains-toolbox-installer uninstall --assumeyes || exit 1
fi
%systemd_preun jetbrains-toolbox-installer.service

%postun
%systemd_postun jetbrains-toolbox-installer.service

%files
%license LICENSE bundled-licenses
%doc README.md
%{_bindir}/jetbrains-toolbox-installer
%{_unitdir}/jetbrains-toolbox-installer.service
%{_mandir}/man1/jetbrains-toolbox-installer.1*

%changelog
* Sat Sep 20 2026 COPR maintainers - 0.1.0-0.1.app3.8.1.88030
- Pin JetBrains Toolbox 3.8.1.88030.

* Wed Sep 16 2026 COPR maintainers - 0.1.0-0.1.app3.8.0.87909
- Automatically queue verified installation of the package-selected Toolbox App.
- Verify the published SHA-256 and unpack the archive without executing it.
- Record the autostart opt-out before the first launch through the owned launcher.
- Remove the verified application on final package removal, preserving user data.
