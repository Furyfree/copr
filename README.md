# COPR packaging

Packaging recipes for the owner's Fedora COPR projects. Application source
stays upstream; each recipe lives in `packages/<name>/<name>.spec`.

| Package | Target | Status |
| --- | --- | --- |
| [Nimbus](packages/nimbus/README.md) | Fedora 44, x86_64 | [0.2.2 recipe; manual builds](https://copr.fedorainfracloud.org/coprs/furyfree/nimbus/builds/) |

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
and Nimbus spec parsing. The actual Nimbus binary build is a separate manual
COPR operation.

Open Actions > COPR > Run workflow on `main` and select `project` to create
the configured project if absent, or verify the required chroots. Existing settings and packages are preserved.

The `build` operation prepares Nimbus's source RPM from the source asset of
its published upstream release, creates/verifies the project, and submits that
exact archive. The native client waits and reports failures. Before a new
version, publish its reviewed Nimbus release and update the spec's `Version`;
then dispatch `build` manually. Pushing either repository does not release or
submit a package.

Publishing runs are serialized. Pull requests, ordinary pushes, and dispatches
from other branches cannot run the publish job. The `furyfree/nimbus` project
has been created through the manual `project` operation. There are no automatic rebuild webhooks or deletion calls.

Project settings are in [.copr/project.toml](.copr/project.toml). Binary builds
use Fedora 44 x86_64 with network access disabled. Source preparation downloads
declared HTTPS sources; it uses temporary storage and leaves the recipe alone.
An SRPM upload supplies COPR with its sources, so COPR does not need a GitHub
credential to clone this packaging repository. Uploads publish their contents;
use only approved release sources.

## Local checks and source RPMs

Prepare the disposable tooling image, then run the complete local gate without
network access or changes to host package configuration:

~~~sh
docker build -t furyfree-copr-tools -f tools/Dockerfile .
docker run --rm --network none --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -v "$PWD:/work:ro" furyfree-copr-tools make check
~~~

With the RPM tools already available, `make check` runs the same gate. To build
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
