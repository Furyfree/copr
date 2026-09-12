# Reviewed 2026-09-08 snapshot and its exact contrib submodule.
%global commit d81fd54feb0d996fdff20dca27eaf0201f7015cc
%global contrib_commit d2109203480a7dfe1dead5f5f8f9f15a9146c90d
%global source_sha256 5af5887fb08fa674079dfb7dc8dcb9d3bbfa786cd82c856a65ef01cc1e5a88a7
%global contrib_sha256 a31f459f2647e6d7c48a302845cd8bf80093a10794ed349d6c4a418425a2d1c6

Name:           blesh
Version:        0.4.0~devel4
Release:        0.1.20260908gitd81fd54%{?dist}
Summary:        Bash line editor with highlighting, suggestions and completion
License:        BSD-3-Clause AND MIT
URL:            https://github.com/akinomyoga/ble.sh
Source0:        %{url}/archive/%{commit}.tar.gz
Source1:        https://github.com/akinomyoga/blesh-contrib/archive/%{contrib_commit}.tar.gz
Source2:        _package.bash
Source3:        vim-airline-themes-LICENSE
BuildArch:      noarch

BuildRequires:  bash
BuildRequires:  gawk
BuildRequires:  git-core
BuildRequires:  make
BuildRequires:  procps-ng
BuildRequires:  ncurses
BuildRequires:  glibc-langpack-en
Requires:       bash
Requires:       coreutils
Requires:       gawk
Requires:       grep
Requires:       sed
Requires:       procps-ng
Requires:       ncurses
Recommends:     bash-completion
Suggests:       fzf

%description
ble.sh enhances interactive Bash with syntax highlighting, history suggestions,
programmable completion and vi editing. Includes the upstream contrib modules,
including fzf menu completion. Enable it from your Bash configuration by sourcing
/usr/share/blesh/ble.sh; installing this package does not edit shell startup files.

%prep
printf '%s  %s\n' '%{source_sha256}' '%{SOURCE0}' | sha256sum --check --strict
printf '%s  %s\n' '%{contrib_sha256}' '%{SOURCE1}' | sha256sum --check --strict
%autosetup -n ble.sh-%{commit}
tar -xzf %{SOURCE1} --strip-components=1 -C contrib

%build
# The archive exports its version metadata; no Git checkout or network is needed.
%{make_build}

%install
%{make_install} PREFIX=%{_prefix} \
  INSDIR_LICENSE=%{buildroot}%{_licensedir}/%{name} \
  INSDIR_DOC=%{buildroot}%{_pkgdocdir}
install -pm 0644 %{SOURCE2} %{buildroot}%{_datadir}/blesh/lib/_package.bash
install -pm 0644 %{SOURCE3} %{buildroot}%{_licensedir}/%{name}/vim-airline-themes-LICENSE
# ble.sh already supports per-user XDG cache and runtime directories.
rmdir %{buildroot}%{_datadir}/blesh/cache.d %{buildroot}%{_datadir}/blesh/run

%check
export HOME="$PWD/test-home"
export USER=builder LOGNAME=builder
export XDG_CONFIG_HOME="$HOME/config" XDG_CACHE_HOME="$HOME/cache"
export XDG_DATA_HOME="$HOME/data" XDG_RUNTIME_DIR="$HOME/run"
mkdir -p "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$XDG_DATA_HOME"
install -d -m 0700 "$XDG_RUNTIME_DIR"
export LANG=C.UTF-8 LC_ALL=C.UTF-8 TERM=xterm-256color
bash out/ble.sh --test
# Exercise the installed hook through ble.sh, including its failure semantics.
status=0
bash %{buildroot}%{_datadir}/blesh/ble.sh --update > update.log 2>&1 || status=$?
test "$status" = 1
grep -Fx 'ble.sh is managed by RPM. Update with: sudo dnf upgrade blesh' update.log

%files
%license %{_licensedir}/%{name}
%doc %{_pkgdocdir}
%{_datadir}/blesh

%changelog
* Sat Sep 12 2026 Patrick Byrne <pby.accounts@byrne.dk> - 0.4.0~devel4-0.1.20260908gitd81fd54
- Package pinned upstream sources and contrib modules, with DNF-owned updates.
