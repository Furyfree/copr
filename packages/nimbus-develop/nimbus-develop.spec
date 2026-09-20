# Source0 is the rolling develop release asset rebuilt by the nimbus
# develop workflow on every push; prepare replaces the version and digest
# with the asset it resolves before building the SRPM.
%global debug_package %{nil}
%global source_sha256 0000000000000000000000000000000000000000000000000000000000000000
%global develop_asset nimbus-0.6.1.dev.00000000000000-vendor.tar.gz

Name:           nimbus
Version:        0.6.1~dev.00000000000000
Release:        0.1%{?dist}
Summary:        Personal Fedora workstation installer and system manager
# LicenseRef-GEANT-CAT: the GÉANT Standard Open Source Software Outward
# Licence in licenses/GEANT-CAT.txt covers the adapted DTU CAT helper; SPDX
# has no identifier for it.
License:        MIT AND BSD-3-Clause AND Apache-2.0 AND Unicode-DFS-2016 AND LicenseRef-GEANT-CAT
URL:            https://github.com/Furyfree/nimbus
Source0:        %{url}/releases/download/develop/%{develop_asset}
ExclusiveArch:  x86_64

BuildRequires:  golang >= 1.26.7
BuildRequires:  git-core
BuildRequires:  gnupg2
BuildRequires:  python3
BuildRequires:  nodejs
Requires:       dnf5
Requires:       rpm
Requires:       flatpak
Requires:       systemd
Requires:       git-core
Requires:       gnupg2
Requires:       sudo

%description
Nimbus coordinates native Fedora tools to inspect and apply a workstation's
selected definitions. The engine reads a separately selected checkout.
Installing this RPM does not initialize or provision the workstation.

%prep
printf '%s  %s\n' '%{source_sha256}' '%{SOURCE0}' | sha256sum --check --strict
%autosetup -n nimbus-%{version}

%build
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go build -mod=vendor -trimpath -buildvcs=false \
  -ldflags '-X github.com/Furyfree/nimbus/internal/version.Engine=%{version}' \
  -o nimbus ./cmd/nimbus

%check
export CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export GOCACHE="$PWD/.gocache"
go test -mod=vendor ./...
./nimbus version
./nimbus validate --checkout .

%install
install -Dpm 0755 nimbus %{buildroot}%{_bindir}/nimbus
install -Dpm 0755 system/root/etc/grub.d/09_nimbus_previous_kernels %{buildroot}/etc/grub.d/09_nimbus_previous_kernels
install -Dpm 0755 system/root/etc/grub.d/30_previous_kernels_nimbus %{buildroot}/etc/grub.d/30_previous_kernels_nimbus
install -Dpm 0755 system/root/etc/grub.d/36_paper_dark %{buildroot}/etc/grub.d/36_paper_dark
install -Dpm 0755 system/root/etc/kernel/install.d/96-nimbus-menu.install %{buildroot}/etc/kernel/install.d/96-nimbus-menu.install
install -Dpm 0755 system/root/usr/lib/dracut/modules.d/40nimbus-plymouth/module-setup.sh %{buildroot}/usr/lib/dracut/modules.d/40nimbus-plymouth/module-setup.sh
install -Dpm 0644 -t %{buildroot}/boot/grub2/themes/nimbus/ system/root/boot/grub2/themes/nimbus/*
install -Dpm 0644 -t %{buildroot}/usr/share/plymouth/themes/nimbus/ system/root/usr/share/plymouth/themes/nimbus/*
install -Dpm 0644 system/root/usr/share/licenses/nimbus-boot-theme/FONT-LICENSE %{buildroot}/usr/share/licenses/nimbus-boot-theme/FONT-LICENSE
# Retain the bundled modules' notices without installing their source trees.
mkdir -p bundled-licenses
find vendor -type f \( -iname 'license*' -o -iname 'copying*' \
  -o -iname 'notice*' -o -iname 'copyright*' \) \
  -exec cp --parents '{}' bundled-licenses/ \;
cp %{_licensedir}/golang/LICENSE bundled-licenses/Go-LICENSE

%files
%license LICENSE bundled-licenses licenses/Unicode-DFS-2016.txt licenses/GEANT-CAT.txt
%license internal/agentproxy/resources/UPSTREAM-LICENSE.txt
%{_bindir}/nimbus
/boot/grub2/themes/nimbus
/etc/grub.d/09_nimbus_previous_kernels
/etc/grub.d/30_previous_kernels_nimbus
/etc/grub.d/36_paper_dark
/etc/kernel/install.d/96-nimbus-menu.install
/usr/lib/dracut/modules.d/40nimbus-plymouth
/usr/share/licenses/nimbus-boot-theme
/usr/share/plymouth/themes/nimbus

%changelog
* Thu Sep 18 2026 Nimbus maintainers - 0.6.1~dev-0.1
- Rolling develop-channel build from the develop branch.

* Mon Sep 14 2026 Nimbus maintainers - 0.5.10-0.1
- Verify DTU certificate labels through native SELinux checks.
- Report specific certificate metadata and labeling failures.

* Mon Sep 14 2026 Nimbus maintainers - 0.5.9-0.1
- Fix terminal process-exit and interrupt-readiness test races.
- Retain the Tailscale initial sign-in and DTU certificate setup changes.

* Mon Sep 14 2026 Nimbus maintainers - 0.5.7-0.1
- Guide initial Tailscale sign-in while preserving local operator permission.
- Add approved, verified DTU eduroam CA installation for NetworkManager.

* Mon Sep 14 2026 Nimbus maintainers - 0.5.6-0.1
- Accept equivalent screen-adjusted lockscreen layouts after Noctalia restart.
- Verify matching saved layouts without repeating the repair.

* Mon Sep 14 2026 Nimbus maintainers - 0.5.5-0.1
- Create managed parent directories during first-time 1Password setup.
- Use a concise change summary and offer a full diff without a pager.

* Mon Sep 14 2026 Nimbus maintainers - 0.5.4-0.1
- Simplify maintenance previews, approvals and final reporting.
- Scope setup inspection and add private versioned run diagnostics.
- Correct status counts for non-package system checks.

* Mon Sep 14 2026 Nimbus maintainers - 0.5.3-0.1
- Enforce declared package sources for installation, repair and upgrades.
- Configure constrained greeter sync during normal approved init and sync.
- Integrate UWSM launches and graceful Noctalia service handling.
- Apply explicitly selected 1Password SSH/Git configuration through Chezmoi.

* Mon Sep 14 2026 Nimbus maintainers - 0.5.2-0.1
- Add guided local-agent proxy setup and owned model refresh after upgrades.
- Add targeted Noctalia lockscreen repair and fix styling after prompts.
- Run adapter tests with Node and retain the upstream patch license notice.

* Sun Sep 13 2026 Nimbus maintainers - 0.5.1-0.1
- Fix setup verification and skip empty update transactions.
- Improve terminal colors, wrapping, previews and final reporting.

* Sun Sep 13 2026 Nimbus maintainers - 0.5.0-0.1
- Engine-first maintenance, guided setup, local evidence and live progress.

* Sun Sep 13 2026 Nimbus maintainers - 0.4.6-0.1
- Add verified native Hyprland plugin and AccountsService picture setup.
- Configure the Noctalia greeter and clarify post-install completion output.

* Sun Sep 13 2026 Nimbus maintainers - 0.4.5-0.1
- Retry verification while Noctalia completes background plugin updates.
- Preserve timeout diagnostics without repeating source updates.

* Sun Sep 13 2026 Nimbus maintainers - 0.4.4-0.1
- Add explicit verification and repair of enabled Noctalia plugin exports.
- Refresh affected plugin sources through Noctalia and wait for runtime files.

* Sat Sep 12 2026 Nimbus maintainers - 0.4.3-0.1
- Add Zeron bootstrap with explicit installer service and lingering disclosures.
- Select Zsh defaults and disable the NVIDIA settings-loader autostart.

* Sat Sep 12 2026 Nimbus maintainers - 0.4.2-0.1
- Reconcile package sources and setup before combined software upgrades.
- Select the verified ble.sh and Noctalia LibrePods COPRs in definitions.

* Sat Sep 12 2026 Nimbus maintainers - 0.4.1-0.1
- Select the default login shell while installing both Bash and Zsh.
- Add explicit Tailscale operator and Proton-CachyOS post-install actions.
- Share power management and include the desktop PDF backend.

* Fri Sep 11 2026 Nimbus maintainers - 0.4.0-0.1
- Update clean Nimbus and Chezmoi repositories during sync.
- Upgrade through Topgrade before restarting the updated engine for sync.
- Add upstream Zed installation and shared browser/private-mode fallbacks.
- Correct VM health checks and conditional session notices.

* Thu Sep 10 2026 Nimbus maintainers - 0.3.1-0.1
- Reuse empty pre-mounted Snapper storage with reviewed, retryable setup.
- Ask for the machine on first installation and restore the Fedora guide.

* Thu Sep 10 2026 Nimbus maintainers - 0.3.0-0.1
- Separate sync from Topgrade upgrades and simplify workstation ownership.
- Add bounded Snapper snapshots and package version constraints.
- Finalize interrupted changes and report Snapper configuration drift.

* Wed Sep 09 2026 Nimbus maintainers - 0.2.3-0.1
- Fix requested package resolution in DNF dependency installation sections.

* Wed Sep 09 2026 Nimbus maintainers - 0.2.2-0.1
- Package the Go modernization and command, planning, and ownership fixes.
- Write state marker schema 3; keep schemas 1 and 2 readable.

* Wed Sep 09 2026 Nimbus maintainers - 0.2.1-0.1
- Package promptless init, toolkit dependencies, and Ghostty recovery fixes.
- Retain the tested plain Hyprland session; defer UWSM adoption.

* Tue Sep 08 2026 Nimbus maintainers - 0.2.0-0.1
- Package system resources, Noctalia login, recovery session, and installer fixes.

* Mon Sep 07 2026 Nimbus maintainers - 0.1.1-0.1
- Package the installation prompt, logging, and repository-convergence fixes.

* Mon Sep 07 2026 Nimbus maintainers - 0.1.0-0.1
- Prepare the first engine RPM; distribution gates remain open.
