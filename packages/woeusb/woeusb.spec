%global script_sha256 0b9e502313c6461346cc4d150459b2e1341d30f9ffda005e160829ad9d5bfffc
%global manpage_sha256 59e53b923b11029fbbe2b4174a8f718a5b92db8ee71b732845bf84c598679921
%global license_sha256 2ca9503d76d1ffab14f599b4741382eec11face60ad1f0d7a41897809003a286

Name:           woeusb
Version:        5.2.4
Release:        1%{?dist}
Summary:        Create bootable Windows USB installation media
License:        GPL-3.0-or-later
URL:            https://github.com/WoeUSB/WoeUSB
Source0:        %{url}/releases/download/v%{version}/woeusb-%{version}.bash
Source1:        https://raw.githubusercontent.com/WoeUSB/WoeUSB/v%{version}/share/man/man1/woeusb.1
Source2:        https://raw.githubusercontent.com/WoeUSB/WoeUSB/v%{version}/LICENSES/GPL-3.0-or-later.txt
BuildArch:      noarch

BuildRequires:  bash
BuildRequires:  coreutils
BuildRequires:  grep
BuildRequires:  sed
Requires:       bash >= 4.3
Requires:       coreutils
Requires:       dosfstools
Requires:       findutils
Requires:       gawk
Requires:       grep
Requires:       grub2-pc-modules
Requires:       grub2-tools
Requires:       ntfs-3g
Requires:       ntfsprogs
Requires:       parted
Requires:       util-linux
Requires:       wget
Requires:       wimlib-utils
Recommends:     7zip

%description
WoeUSB creates bootable Microsoft Windows installation USB media from an
installation disc or ISO image. It supports legacy PC and UEFI booting, FAT32
and NTFS target filesystems, and can split oversized install.wim archives for
FAT32 media.

%prep
printf '%s  %s\n' '%{script_sha256}' '%{SOURCE0}' | sha256sum --check --strict
printf '%s  %s\n' '%{manpage_sha256}' '%{SOURCE1}' | sha256sum --check --strict
printf '%s  %s\n' '%{license_sha256}' '%{SOURCE2}' | sha256sum --check --strict

%build

%install
install -Dpm 0755 %{SOURCE0} %{buildroot}%{_bindir}/woeusb
install -Dpm 0644 %{SOURCE1} %{buildroot}%{_mandir}/man1/woeusb.1
sed -i 's/@@WOEUSB_VERSION@@/%{version}/g' %{buildroot}%{_mandir}/man1/woeusb.1
install -Dpm 0644 %{SOURCE2} %{buildroot}%{_licensedir}/%{name}/GPL-3.0-or-later.txt

%check
test "$(bash %{SOURCE0} --version)" = "%{version}"
bash %{SOURCE0} --no-color --help | grep -F -- '--device'

%files
%license %{_licensedir}/%{name}/GPL-3.0-or-later.txt
%{_bindir}/woeusb
%{_mandir}/man1/woeusb.1*

%changelog
* Fri Sep 18 2026 Patrick Byrne <pby.accounts@byrne.dk> - 5.2.4-1
- Package the upstream WoeUSB 5.2.4 command for Fedora 44.
- Include FAT32 WIM splitting and the GRUB i386-pc modules required by --device.
