%global debug_package %{nil}
%global app_version 1.1.17

Name:           github-copilot-installer
Version:        0.3.0
Release:        0.1.app%{app_version}%{?dist}
Summary:        Automatically install the selected official GitHub Copilot app
License:        MIT AND BSD-3-Clause
URL:            https://github.com/Furyfree/copr
Source0:        github-copilot-installer-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
BuildRequires:  systemd-rpm-macros
Requires:       dnf5
Requires:       rpm
Requires:       systemd
Requires:       util-linux >= 2.40
%{?systemd_requires}

%description
Installing or upgrading this package schedules installation of Copilot
%{app_version} directly from GitHub. A one-shot systemd job waits for DNF's
transaction lock, checks the package-pinned SHA-256 and native RPM identity,
and installs the official RPM through DNF. The proprietary app is not bundled.
The job finishes separately from the transaction that installs this package.

%prep
%autosetup -n github-copilot-installer-%{version}

%build
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go build -mod=vendor -trimpath -buildvcs=false -o github-copilot-installer ./cmd/github-copilot-installer

%check
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go test -mod=vendor ./...
./github-copilot-installer version

%install
install -Dpm 0755 github-copilot-installer %{buildroot}%{_bindir}/github-copilot-installer
install -Dpm 0644 github-copilot-installer.1 %{buildroot}%{_mandir}/man1/github-copilot-installer.1
install -Dpm 0644 github-copilot-installer.service %{buildroot}%{_unitdir}/github-copilot-installer.service
mkdir -p bundled-licenses
if test -d vendor; then
    find vendor -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp --parents '{}' bundled-licenses/ \;
fi
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%posttrans
if test -d /run/systemd/system; then
    systemctl daemon-reload || exit 1
    # A freshly installed unit may not be loaded yet and has no failure to reset.
    systemctl reset-failed github-copilot-installer.service 2>/dev/null || :
    systemctl --no-block restart github-copilot-installer.service || exit 1
    echo 'Copilot %{app_version} installation queued after DNF. Check: systemctl status github-copilot-installer.service'
else
    echo 'Copilot installation needs a running systemd host. Run dnf reinstall github-copilot-installer after boot.' >&2
fi

%preun
%systemd_preun github-copilot-installer.service

%postun
%systemd_postun github-copilot-installer.service

%files
%license LICENSE bundled-licenses
%doc README.md
%{_bindir}/github-copilot-installer
%{_mandir}/man1/github-copilot-installer.1*
%{_unitdir}/github-copilot-installer.service

%changelog
* Fri Sep 11 2026 COPR maintainers - 0.3.0-0.1.app1.1.17
- Pin the selected app version and digest in the signed helper package.
- Automatically queue native app installation after package installs and upgrades.

* Thu Sep 10 2026 COPR maintainers - 0.2.0-0.1
- Build the Go helper and its offline lifecycle tests from a dedicated source bundle.
- Separate unprivileged artifact preparation from verified offline application.
- Refuse downgrades and constrain downloads to stable official GitHub assets.

* Thu Sep 10 2026 COPR maintainers - 0.1.2-0.1
- Maintain the MIT helper locally, with offline status and HTTPS redirects.
