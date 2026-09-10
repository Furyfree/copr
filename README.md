# COPR packaging

Packaging recipes for the owner's Fedora COPR projects. Each recipe lives in
`packages/<name>/<name>.spec`. Application source stays upstream; the Copilot
installer helper is maintained locally alongside its recipe.

| Package | Target | Status |
| --- | --- | --- |
| [Nimbus](packages/nimbus/README.md) | Fedora 44, x86_64 | [0.2.3 recipe; manual builds](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/builds/) |
| [Voxtype](packages/voxtype/README.md) | Fedora 44, x86_64 | [1.0.1-0.2 CPU build published](https://copr.fedorainfracloud.org/coprs/furyfree/voxtype/build/10968949/) |
| [GitHub Copilot installer](packages/github-copilot-installer/README.md) | Fedora 44, x86_64 | [0.1.2-0.1 published](https://copr.fedorainfracloud.org/coprs/furyfree/github-copilot-installer/build/10968946/); app downloaded separately |

## Account setup

1. Sign in to [Fedora COPR](https://copr.fedorainfracloud.org/) with a Fedora
   account, then open its [API page](https://copr.fedorainfracloud.org/api/).
2. In `Furyfree/copr` under Settings > Secrets and variables > Actions, set
   the variable `COPR_OWNER` to that account's username.
3. Set the Actions secret `COPR_CONFIG` to the complete generated `[copr-cli]`
   configuration. Never commit it or paste it into logs.

Actions installs the native COPR tools in its container. A local COPR login
or host package installation is not required for this route. The workflow
checks that the configuration selects the official service and matching owner.
It writes credentials to a temporary mode-0600 file and removes that file
on exit. Rotate the secret when its token expires.

## Workflow

Pull requests and pushes to `main` run **Check packaging code and spec syntax**
without COPR credentials. This covers regressions, a real fixture SRPM build,
and all package spec parsing. COPR binary builds are separate manual
operations, one selected package per run.

After these changes are merged to `main`, open Actions > COPR > Run workflow
and select one `package` and an `operation`:

| Package selection | COPR project | Source preparation |
| --- | --- | --- |
| `nimbus` | `<owner>/nimbus` | Published, checksum-pinned Nimbus release |
| `voxtype` | `<owner>/voxtype` | Signed upstream release and locked Cargo vendoring |
| `github-copilot-installer` | `<owner>/github-copilot-installer` | Local MIT helper source from the selected `main` commit |

Choose `project` to create or verify only that project's Fedora 44 x86_64
configuration. Choose `build` to prepare and upload only that package's SRPM;
it also creates/verifies the project, so a separate `project` run is optional.
The native client waits for the result and reports build failures. The
publisher rejects an SRPM whose native package name differs from the selection.

With GitHub CLI authenticated, use the Just recipes:

~~~sh
just publish voxtype
just publish github-copilot-installer
just publish nimbus
just project voxtype
~~~

A package selection is required. These commands dispatch remote `main`, so
merge reviewed package changes first. `publish` selects `operation=build`;
`project` creates/verifies the selected COPR project without building.

The equivalent direct GitHub CLI command for Voxtype is:

~~~sh
gh workflow run copr.yml --repo Furyfree/copr --ref main \
  -f operation=build -f package=voxtype
~~~

Use `package=nimbus` or `package=github-copilot-installer` for the other
packages. Each project has its own signing key; verify the resulting key and
signed RPM before adding a new project to Nimbus. The Copilot project ships
only the installer helper, not the proprietary app. Package readiness and
real installation tests remain described in each package's README.

Runs for the same package are serialized; different packages can publish
independently. Pull requests, ordinary pushes, and dispatches from other
branches cannot publish. There are no automatic rebuild webhooks or deletion
calls. Creating/building one project never changes another project's settings.
The existing `furyfree/nimbus` project and its key remain in use.

Project settings are in [.copr/projects.toml](.copr/projects.toml). Binary
builds use Fedora 44 x86_64 with networking disabled. Source preparation runs
without COPR credentials and may download declared HTTPS sources. Credentials
are exposed only to the final project/upload step. An SRPM supplies COPR with
its sources, so COPR needs no GitHub credential to clone this repository.
Uploads publish their contents; use only reviewed sources.

Before a new version, update that package's version, release, and source
verification data as applicable. Nimbus needs its upstream release published
first. Voxtype verifies its upstream signature and checksum before vendoring.
The helper is maintained here; update its source and version together.
Pushing a change alone never publishes a package.

## Local checks and source RPMs

Prepare the disposable tooling image, then run the complete local gate without
network access or changes to host package configuration:

~~~sh
docker build -t furyfree-copr-tools -f tools/Dockerfile .
docker run --rm --network none --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -v "$PWD:/work:ro" furyfree-copr-tools just check
~~~

With Just and the RPM tools available, `just check` runs the same gate. To build
a source RPM after supplying the package's declared sources:

~~~sh
python3 scripts/srpm.py --spec packages/nimbus/nimbus.spec --outdir /tmp/nimbus-srpm
~~~

The shared `.copr/Makefile` also implements COPR's native `make_srpm` contract:
invoke its `srpm` target from a package directory with `spec` and `outdir`.
It requires installed RPM tools and never installs system dependencies itself.
Source RPM creation does not compile the engine or verify its build requirements;
the subsequent Fedora binary build is a separate gate.

## Reference

The structure was informed by Chris Titus Tech's
[copr-fedora](https://github.com/ChrisTitusTech/copr-fedora) at commit
`db775cf` and the [COPR documentation](https://docs.copr.fedorainfracloud.org/user_documentation.html).
The inspected checkout has no license declaration; its automation and tests
were not copied into this MIT-licensed repository. This implementation uses
native RPM tools and the COPR client directly.
