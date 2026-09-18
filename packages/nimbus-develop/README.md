# nimbus-develop

The develop-channel Nimbus engine, built from the rolling `develop` source.

The nimbus `develop` workflow rebuilds and republishes the vendored archive on
every push to the `develop` branch, under the GitHub release tag `develop`.
`coprctl prepare nimbus-develop` resolves that release metadata, takes the
version and digest from the asset name and `SHA256SUMS`, substitutes both into
a copy of `nimbus-develop.spec`, and builds the SRPM. The RPM package is named
`nimbus`; the COPR project `nimbus-develop` keeps it separate from stable.

Versioning: `0.6.0~dev.<YYYYMMDD>git<sha>`, which RPM orders above `0.5.x` and
below a released `0.6.0`, so a machine on stable upgrades to develop and later
upgrades back to a released stable version.

Publish with `just project nimbus-develop` and `just build nimbus-develop`
(through the copr repository workflow), or the `coprctl` commands directly.
