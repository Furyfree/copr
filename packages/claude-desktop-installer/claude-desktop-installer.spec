%global debug_package %{nil}
%global app_version pin

Name:           claude-desktop-installer
Version:        0.1.1
Release:        0.1.app%{app_version}%{?dist}
Summary:        Install the selected official Claude Desktop Linux application
License:        MIT AND BSD-3-Clause
URL:            https://github.com/Furyfree/copr
Source0:        claude-desktop-installer-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
BuildRequires:  systemd-rpm-macros
Requires:       systemd
Requires:       util-linux >= 2.40
%{?systemd_requires}
Requires:       gnupg2
Requires:       binutils
Requires:       xz
Requires:       gtk3
Requires:       libnotify
Requires:       nss
Requires:       xdg-utils
Requires:       at-spi2-atk
Requires:       libdrm
Requires:       mesa-libgbm
Requires:       libxcb
Requires:       libsecret
Requires:       gvfs
Requires:       libXtst
Requires:       libuuid
Requires:       pipewire-libs
Requires:       alsa-lib
Requires:       ca-certificates
Requires:       xdg-desktop-portal
Requires:       (xdg-desktop-portal-gtk or xdg-desktop-portal-gnome or xdg-desktop-portal-kde)
Recommends:     gnome-keyring
Recommends:     bubblewrap
Recommends:     socat
Recommends:     openssh-clients
Recommends:     qemu-system-x86-core
Recommends:     edk2-ovmf
Recommends:     virtiofsd

%description
Installing or upgrading this helper queues a one-shot systemd job after DNF.
The job downloads Claude Desktop %{app_version} from Anthropic, verifies the
signed repository metadata and package-selected SHA-256, and creates desktop
integration. COPR contains only this open-source helper, not the application.
Fedora integration is unofficial; Anthropic supports Debian and Ubuntu.
Removing the helper removes only the verified application and integration,
preserving user settings. Cowork needs working virtualization and a paid plan.

%prep
%autosetup -n claude-desktop-installer-%{version}

%build
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go build -mod=vendor -trimpath -buildvcs=false -o claude-desktop-installer ./cmd/claude-desktop-installer

%check
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go test -mod=vendor ./...
./claude-desktop-installer --version

%install
install -Dpm 0755 claude-desktop-installer %{buildroot}%{_bindir}/claude-desktop-installer
install -Dpm 0644 claude-desktop-installer.service %{buildroot}%{_unitdir}/claude-desktop-installer.service
install -Dpm 0644 claude-desktop-installer.1 %{buildroot}%{_mandir}/man1/claude-desktop-installer.1
mkdir -p bundled-licenses
if test -d vendor; then
    find vendor -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp --parents '{}' bundled-licenses/ \;
fi
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%posttrans
if test -d /run/systemd/system; then
    systemctl daemon-reload || exit 1
    # A freshly installed unit may not be loaded yet and has no failure to reset.
    systemctl reset-failed claude-desktop-installer.service 2>/dev/null || :
    systemctl --no-block restart claude-desktop-installer.service || exit 1
    echo 'Claude Desktop %{app_version} installation queued after DNF. Check: systemctl status claude-desktop-installer.service'
else
    echo 'Claude installation needs a running systemd host. Run dnf reinstall claude-desktop-installer after boot.' >&2
fi

%preun
if [ "$1" -eq 0 ]; then
    if test -d /run/systemd/system; then
        systemctl stop claude-desktop-installer.service || exit 1
    fi
    # Keep the helper installed when ownership verification or cleanup fails.
    %{_bindir}/claude-desktop-installer uninstall --assumeyes || exit 1
fi
%systemd_preun claude-desktop-installer.service

%postun
%systemd_postun claude-desktop-installer.service

%files
%license LICENSE bundled-licenses
%doc README.md
%{_bindir}/claude-desktop-installer
%{_unitdir}/claude-desktop-installer.service
%{_mandir}/man1/claude-desktop-installer.1*
