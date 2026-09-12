# ble.sh package-manager hook; updates remain explicit DNF transactions.
_ble_base_package_type=rpm
function ble/base/package:rpm/update {
    printf '%s\n' 'ble.sh is managed by RPM. Update with: sudo dnf upgrade blesh' >&2
    return 1
}
