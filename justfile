# The complete local gate.
check:
    PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
    rpmspec -P packages/nimbus/nimbus.spec > /dev/null
    rpmspec -P packages/voxtype/voxtype.spec > /dev/null
    rpmspec -P packages/github-copilot-installer/github-copilot-installer.spec > /dev/null
    git diff --check

# Publish one package from remote main; merge reviewed changes first.
publish package: (_dispatch "build" package)

# Create or verify one COPR project without building.
project package: (_dispatch "project" package)

[private]
_dispatch operation package:
    @case {{ quote(package) }} in nimbus|voxtype|github-copilot-installer) ;; *) echo 'Choose nimbus, voxtype, or github-copilot-installer' >&2; exit 2 ;; esac
    gh workflow run copr.yml --repo Furyfree/copr --ref main -f {{ quote("operation=" + operation) }} -f {{ quote("package=" + package) }}
