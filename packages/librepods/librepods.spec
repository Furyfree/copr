%global commit 4ed49df0b301ac3e6fba9c81dfbbb6726cc52201
%global source_sha256 8bbf5abd61b6002adc556f575b42226be9088e12afd26bafaf989a601bb75d74

Name:           librepods
Version:        20260826
Release:        1%{?dist}
Summary:        AirPods daemon with Noctalia status and control integration
License:        GPL-3.0-only AND MIT
URL:            https://github.com/harveywuk/librepods
Source0:        https://github.com/harveywuk/librepods/archive/%{commit}/librepods-%{commit}.tar.gz
Source1:        librepods.service
ExclusiveArch:  x86_64
BuildRequires:  gcc-c++
BuildRequires:  cmake
BuildRequires:  ninja-build
BuildRequires:  qt6-qtbase-devel
BuildRequires:  qt6-qtdeclarative-devel
BuildRequires:  qt6-qtconnectivity-devel
BuildRequires:  qt6-qttools-devel
BuildRequires:  openssl-devel
BuildRequires:  pkgconfig(libpulse)
BuildRequires:  desktop-file-utils
BuildRequires:  systemd-rpm-macros
Requires:       bluez
Requires:       qt6-qtdeclarative
%{?systemd_requires}

%description
Pinned snapshot of the LibrePods fork used by the Noctalia AirPods widget.
It provides a private status file, extended librepods-ctl commands, and a
headless daemon. The graphical application remains available separately.
The user service follows Fedora presets and is not started on installation.

%prep
printf '%s  %s\n' '%{source_sha256}' '%{SOURCE0}' | sha256sum --check --strict
%autosetup -n librepods-%{commit}

%build
%cmake -DBUILD_TESTING=ON
%cmake_build

%check
export QT_QPA_PLATFORM=offscreen
test_home="$PWD/test-home"
export XDG_CONFIG_HOME="$test_home/config" XDG_STATE_HOME="$test_home/state"
export XDG_CACHE_HOME="$test_home/cache" XDG_RUNTIME_DIR="$test_home/runtime"
mkdir -p "$XDG_CONFIG_HOME" "$XDG_STATE_HOME" "$XDG_CACHE_HOME" "$XDG_RUNTIME_DIR"
chmod 0700 "$test_home" "$XDG_RUNTIME_DIR"
%ctest
desktop-file-validate assets/me.kavishdevar.librepods.desktop

%install
%cmake_install
# The fork's local-install unit points at ~/.local; RPM owns the system user unit.
rm -f %{buildroot}%{_datadir}/systemd/user/librepods.service
install -Dpm 0644 %{SOURCE1} %{buildroot}%{_userunitdir}/librepods.service

%post
%systemd_user_post librepods.service

%preun
%systemd_user_preun librepods.service

%postun
%systemd_user_postun_with_restart librepods.service

%files
%license LICENSE thirdparty/QR-Code-generator/qrcodegen.hpp
%doc README.md UPSTREAM.md
%{_bindir}/librepods
%{_bindir}/librepods-ctl
%{_userunitdir}/librepods.service
%{_datadir}/applications/me.kavishdevar.librepods.desktop
%{_datadir}/icons/hicolor/scalable/apps/librepods.svg
%{_datadir}/openpods/translations/

%changelog
* Sat Sep 12 2026 Patrick Byrne <pby.accounts@byrne.dk> - 20260826-1
- Package the pinned Noctalia-compatible fork with its headless user service.
