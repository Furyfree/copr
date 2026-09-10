Name:           github-copilot-installer
Version:        0.1.2
Release:        0.1%{?dist}
Summary:        Installer helper for the official GitHub Copilot app
License:        MIT
URL:            https://github.com/Furyfree/copr
Source0:        github-copilot-installer
Source1:        github-copilot-installer.1
Source2:        LICENSE
Source3:        README.md
Source4:        test-installer.sh
BuildArch:      noarch

BuildRequires:  bash
BuildRequires:  coreutils
BuildRequires:  jq
Requires:       bash
Requires:       coreutils
Requires:       curl
Requires:       dnf5
Requires:       jq
Requires:       rpm

%description
An open-source helper that downloads the official GitHub Copilot RPM directly
from GitHub on explicit invocation, checks its published SHA-256 digest and
package identity, and delegates installation, updates, and removal to DNF.
The proprietary application is not included. Installing this helper does not
download or install the application and does not enable background updates.

%prep
%setup -q -c -T
cp -p %{SOURCE0} %{SOURCE1} %{SOURCE2} %{SOURCE3} %{SOURCE4} .
chmod 0755 github-copilot-installer test-installer.sh

%build

%install
install -Dpm 0755 github-copilot-installer \
    %{buildroot}%{_bindir}/github-copilot-installer
install -Dpm 0644 github-copilot-installer.1 \
    %{buildroot}%{_mandir}/man1/github-copilot-installer.1

%check
bash -n github-copilot-installer
bash test-installer.sh

%files
%license LICENSE
%doc README.md
%{_bindir}/github-copilot-installer
%{_mandir}/man1/github-copilot-installer.1*

%changelog
* Thu Sep 10 2026 COPR maintainers - 0.1.2-0.1
- Maintain the MIT helper locally, with offline status and HTTPS redirects.
