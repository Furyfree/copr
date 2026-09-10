%global debug_package %{nil}

Name:           github-copilot-installer
Version:        0.2.0
Release:        0.1%{?dist}
Summary:        Installer helper for the official GitHub Copilot app
License:        MIT AND BSD-3-Clause
URL:            https://github.com/Furyfree/copr
Source0:        github-copilot-installer-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
Requires:       dnf5
Requires:       rpm

%description
An open-source helper that downloads the official GitHub Copilot RPM directly
from GitHub on explicit invocation, checks its published SHA-256 digest and
package identity, and delegates installation, updates, and removal to DNF.
The proprietary application is not included. Installing this helper does not
download or install the application and does not enable background updates.

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
mkdir -p bundled-licenses
if test -d vendor; then
    find vendor -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp --parents '{}' bundled-licenses/ \;
fi
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%files
%license LICENSE bundled-licenses
%doc README.md
%{_bindir}/github-copilot-installer
%{_mandir}/man1/github-copilot-installer.1*

%changelog
* Thu Sep 10 2026 COPR maintainers - 0.2.0-0.1
- Build the Go helper and its offline lifecycle tests from a dedicated source bundle.
- Separate unprivileged artifact preparation from verified offline application.
- Refuse downgrades and constrain downloads to stable official GitHub assets.

* Thu Sep 10 2026 COPR maintainers - 0.1.2-0.1
- Maintain the MIT helper locally, with offline status and HTTPS redirects.
