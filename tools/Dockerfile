FROM registry.fedoraproject.org/fedora:44
RUN dnf5 -y install git make python3 rpm-build rpmdevtools copr-cli python3-copr \
    && dnf5 clean all
WORKDIR /work
