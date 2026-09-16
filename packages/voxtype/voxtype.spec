# coprctl prepare supplies the locked, vendored source and its exact digest.
%global debug_package %{nil}
%global source_sha256 UNPREPARED

Name:           voxtype
Version:        1.0.1
Release:        0.3%{?dist}
Summary:        Push-to-talk voice transcription for Wayland
License:        MIT AND Apache-2.0 AND ISC AND BSD-3-Clause AND CC0-1.0 AND CDLA-Permissive-2.0 AND MPL-2.0 AND Unicode-3.0 AND Unlicense AND Zlib AND LicenseRef-Fedora-Public-Domain
URL:            https://github.com/peteonrails/voxtype
Source0:        voxtype-%{version}-vendor.tar.gz
ExclusiveArch:  x86_64
BuildRequires:  cargo
BuildRequires:  rust
BuildRequires:  gcc-c++
BuildRequires:  clang-devel
BuildRequires:  alsa-lib-devel
BuildRequires:  cmake
BuildRequires:  glslc
BuildRequires:  vulkan-headers
BuildRequires:  vulkan-loader-devel
BuildRequires:  systemd-rpm-macros
%{?systemd_requires}
Requires:       pipewire-alsa
Requires:       wtype
Requires:       wl-clipboard
Requires:       libnotify
Requires:       curl

%description
Voxtype transcribes speech using Whisper. Inference uses AVX2/FMA CPU kernels
and, when a Vulkan driver is present, the GPU; without one it falls back to
the CPU. This build requires an x86-64-v3 CPU (Intel Haswell/AMD Excavator or
newer). Model downloads and compositor keybindings are configured separately
by the user. This package follows Fedora's service preset policy and does not
start services, download models, or grant input-device access on first
installation.

%prep
printf '%s  %s\n' '%{source_sha256}' '%{SOURCE0}' | sha256sum --check --strict
%autosetup -n voxtype-%{version}

%build
export CARGO_HOME="$PWD/.cargo-home"
# Deterministic x86-64-v3 kernels (AVX2/FMA, no AVX-512): about 8x faster
# Whisper inference than the plain baseline, independent of the builder CPU.
export GGML_NATIVE=OFF GGML_AVX512=OFF
export CFLAGS='%{optflags} -march=x86-64-v3' CXXFLAGS='%{optflags} -march=x86-64-v3'
export CMAKE_C_FLAGS="$CFLAGS" CMAKE_CXX_FLAGS="$CXXFLAGS"
export RUSTFLAGS='-C target-cpu=x86-64-v3'
cargo build --release --frozen --offline --no-default-features --features gpu-vulkan --bin voxtype

%check
export CARGO_HOME="$PWD/.cargo-home"
export GGML_NATIVE=OFF GGML_AVX512=OFF
export CFLAGS='%{optflags} -march=x86-64-v3' CXXFLAGS='%{optflags} -march=x86-64-v3'
export CMAKE_C_FLAGS="$CFLAGS" CMAKE_CXX_FLAGS="$CXXFLAGS"
export RUSTFLAGS='-C target-cpu=x86-64-v3'
cargo test --release --frozen --offline --no-default-features --features gpu-vulkan --lib
./target/release/voxtype --version
./target/release/voxtype --help > /dev/null

%install
install -Dpm 0755 target/release/voxtype %{buildroot}%{_bindir}/voxtype
install -Dpm 0644 config/default.toml %{buildroot}%{_sysconfdir}/voxtype/config.toml
install -Dpm 0644 packaging/systemd/voxtype.service %{buildroot}%{_userunitdir}/voxtype.service
install -Dpm 0644 packaging/completions/voxtype.bash %{buildroot}%{_datadir}/bash-completion/completions/voxtype
install -Dpm 0644 packaging/completions/voxtype.zsh %{buildroot}%{_datadir}/zsh/site-functions/_voxtype
install -Dpm 0644 packaging/completions/voxtype.fish %{buildroot}%{_datadir}/fish/vendor_completions.d/voxtype.fish
for manual in target/release/build/voxtype-*/out/man/*.1; do
    install -Dpm 0644 "$manual" "%{buildroot}%{_mandir}/man1/${manual##*/}"
done
mkdir -p bundled-licenses
find vendor -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp --parents '{}' bundled-licenses/ \;

%post
%systemd_user_post voxtype.service

%preun
%systemd_user_preun voxtype.service

%postun
%systemd_user_postun_with_restart voxtype.service

%files
%license LICENSE bundled-licenses
%doc README.md docs/INSTALL.md
%{_bindir}/voxtype
%{_mandir}/man1/voxtype*.1*
%config(noreplace) %{_sysconfdir}/voxtype/config.toml
%{_userunitdir}/voxtype.service
%{_datadir}/bash-completion/completions/voxtype
%{_datadir}/zsh/site-functions/_voxtype
%{_datadir}/fish/vendor_completions.d/voxtype.fish

%changelog
* Wed Sep 16 2026 COPR maintainers - 1.0.1-0.3
- Build Whisper inference with x86-64-v3 (AVX2/FMA) kernels; the previous
  baseline build ran SSE-only and transcribed about 8x slower.
- Enable the Vulkan GPU backend with automatic CPU fallback.

* Thu Sep 10 2026 COPR maintainers - 1.0.1-0.2
- Handle user-service removal and restart on upgrade using Fedora macros.

* Wed Sep 09 2026 Nimbus maintainers - 1.0.1-0.1
- Prepare a signed-source CPU build with locked offline Cargo dependencies.
