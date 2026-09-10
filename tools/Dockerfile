FROM registry.fedoraproject.org/fedora:44
RUN dnf5 -y install git make just python3 rpm-build rpmdevtools copr-cli python3-copr jq \
    systemd systemd-rpm-macros python3-pyyaml \
    && dnf5 clean all
WORKDIR /work
