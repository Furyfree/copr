#!/usr/bin/env bash

set -euo pipefail

TEST_ROOT="$(mktemp -d -t github-copilot-tests.XXXXXXXX)"
readonly TEST_ROOT
INSTALLER="$(
    cd "$(dirname "${BASH_SOURCE[0]}")"
    pwd
)/github-copilot-installer"
readonly INSTALLER
readonly STUB_DIRECTORY="$TEST_ROOT/stubs"
readonly STATE_FILE="$TEST_ROOT/installed"
readonly DNF_LOG="$TEST_ROOT/dnf.log"
readonly CURL_LOG="$TEST_ROOT/curl.log"
readonly FAKE_RPM_CONTENT="fake GitHub Copilot RPM"
FAKE_RPM_SHA256="$(
    printf '%s' "$FAKE_RPM_CONTENT" | sha256sum | awk '{print $1}'
)"
readonly FAKE_RPM_SHA256

passed=0

cleanup() {
    rm -rf -- "$TEST_ROOT"
}

trap cleanup EXIT HUP INT TERM

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

assert_contains() {
    local file="$1"
    local expected="$2"

    grep -Fq -- "$expected" "$file" ||
        fail "expected '$expected' in $file"
}

assert_not_exists() {
    [[ ! -e "$1" ]] || fail "unexpected path exists: $1"
}

reset_case() {
    rm -f -- "$STATE_FILE" "$DNF_LOG" "$CURL_LOG"
    export TEST_UID=0
    export TEST_ARCH=x86_64
    export TEST_RELEASE_VERSION=1.1.1
    export TEST_ASSET_URL=""
    export TEST_DIGEST="$FAKE_RPM_SHA256"
    export TEST_RPM_NAME=github
    export TEST_RPM_VERSION=1.1.1
    export TEST_RPM_RELEASE=1
    export TEST_RPM_ARCH=x86_64
    export TEST_RPM_LICENSE=Proprietary
    export TEST_RPM_SUMMARY="Tauri Copilot Application"
    export TEST_CURL_FAIL=0
    export TEST_DNF_FAIL=0
    export TEST_CHECKSIG_FAIL=0
    export TEST_SIGNAL=0
}

run_success() {
    local name="$1"
    shift
    local output="$TEST_ROOT/${name}.out"

    if ! "$@" >"$output" 2>&1; then
        sed -n '1,160p' "$output" >&2
        fail "$name unexpectedly failed"
    fi
    passed=$((passed + 1))
}

run_failure() {
    local name="$1"
    shift
    local output="$TEST_ROOT/${name}.out"

    if "$@" >"$output" 2>&1; then
        sed -n '1,160p' "$output" >&2
        fail "$name unexpectedly succeeded"
    fi
    passed=$((passed + 1))
}

mkdir -p "$STUB_DIRECTORY"

cat >"$STUB_DIRECTORY/stub-command" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

command_name="$(basename "$0")"

case "$command_name" in
id)
    [[ "${1:-}" == "-u" ]] || exit 2
    printf '%s\n' "${TEST_UID:-0}"
    ;;
uname)
    [[ "${1:-}" == "-m" ]] || exit 2
    printf '%s\n' "${TEST_ARCH:-x86_64}"
    ;;
curl)
    [[ "${TEST_CURL_FAIL:-0}" == "0" ]] || exit 22
    if [[ "${TEST_SIGNAL:-0}" == "1" ]]; then
        kill -TERM "$PPID"
    fi
    output=""
    url=""
    while (($# > 0)); do
        case "$1" in
        --output)
            shift
            output="$1"
            ;;
        https://*)
            url="$1"
            ;;
        esac
        shift
    done
    [[ -n "$output" && -n "$url" ]] || exit 2
    printf '%s\n' "$url" >>"$TEST_CURL_LOG"
    if [[ "$url" == *"/releases/latest" || "$url" == *"/releases/tags/"* ]]; then
        version="${TEST_RELEASE_VERSION:-1.1.1}"
        asset_url="${TEST_ASSET_URL:-}"
        if [[ -z "$asset_url" ]]; then
            asset_url="https://github.com/github/app/releases/download/v${version}/GitHub-Copilot-linux-x64.rpm"
        fi
        jq -n \
            --arg tag "v${version}" \
            --arg url "$asset_url" \
            --arg digest "sha256:${TEST_DIGEST}" \
            '{
                tag_name: $tag,
                assets: [{
                    name: "GitHub-Copilot-linux-x64.rpm",
                    browser_download_url: $url,
                    digest: $digest
                }]
            }' >"$output"
    else
        printf '%s' 'fake GitHub Copilot RPM' >"$output"
    fi
    ;;
rpm)
    case "${1:-}" in
    --checksig)
        [[ "${TEST_CHECKSIG_FAIL:-0}" == "0" ]]
        ;;
    -qp)
        printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
            "${TEST_RPM_NAME:-github}" \
            "${TEST_RPM_VERSION:-1.1.1}" \
            "${TEST_RPM_RELEASE:-1}" \
            "${TEST_RPM_ARCH:-x86_64}" \
            "${TEST_RPM_LICENSE:-Proprietary}" \
            "${TEST_RPM_SUMMARY:-Tauri Copilot Application}"
        ;;
    -q)
        [[ -f "$TEST_STATE_FILE" ]] || exit 1
        printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
            "${TEST_RPM_NAME:-github}" \
            "${TEST_RPM_VERSION:-1.1.1}" \
            "${TEST_RPM_RELEASE:-1}" \
            "${TEST_RPM_ARCH:-x86_64}" \
            "${TEST_RPM_LICENSE:-Proprietary}" \
            "${TEST_RPM_SUMMARY:-Tauri Copilot Application}"
        ;;
    *)
        exit 2
        ;;
    esac
    ;;
dnf)
    printf '%s\n' "$*" >>"$TEST_DNF_LOG"
    [[ "${TEST_DNF_FAIL:-0}" == "0" ]] || exit 1
    if [[ " $* " == *" install "* ]]; then
        : >"$TEST_STATE_FILE"
    elif [[ " $* " == *" remove github "* ]]; then
        rm -f -- "$TEST_STATE_FILE"
    else
        exit 2
    fi
    ;;
*)
    exit 127
    ;;
esac
EOF
chmod +x "$STUB_DIRECTORY/stub-command"
for command in curl dnf id rpm uname; do
    ln -s stub-command "$STUB_DIRECTORY/$command"
done

export PATH="$STUB_DIRECTORY:/usr/bin:/bin"
export TEST_STATE_FILE="$STATE_FILE"
export TEST_DNF_LOG="$DNF_LOG"
export TEST_CURL_LOG="$CURL_LOG"
export TMPDIR="$TEST_ROOT"

reset_case
run_success version "$INSTALLER" version
assert_contains "$TEST_ROOT/version.out" "github-copilot-installer 0.1.2"

reset_case
run_success help "$INSTALLER" help
assert_contains "$TEST_ROOT/help.out" "Usage:"

reset_case
run_success install "$INSTALLER" install --assumeyes
assert_contains "$DNF_LOG" "-y install"
assert_contains "$TEST_ROOT/install.out" "GitHub Copilot 1.1.1-1 is installed."

reset_case
run_failure update_missing "$INSTALLER" update
assert_contains "$TEST_ROOT/update_missing.out" "is not installed"
assert_not_exists "$DNF_LOG"

reset_case
export TEST_UID=1000
run_failure non_root "$INSTALLER" install
assert_contains "$TEST_ROOT/non_root.out" "run it with sudo"
assert_not_exists "$CURL_LOG"

reset_case
TEST_DIGEST="$(
    printf '%064d' 0
)"
export TEST_DIGEST
run_failure bad_digest "$INSTALLER" install
assert_contains "$TEST_ROOT/bad_digest.out" "does not match"
assert_not_exists "$DNF_LOG"

reset_case
export TEST_ASSET_URL="https://example.invalid/GitHub-Copilot-linux-x64.rpm"
run_failure bad_url "$INSTALLER" install
assert_contains "$TEST_ROOT/bad_url.out" "not the expected official"
assert_not_exists "$DNF_LOG"

reset_case
export TEST_RPM_NAME=unexpected
run_failure bad_rpm "$INSTALLER" install
assert_contains "$TEST_ROOT/bad_rpm.out" "unexpected RPM package name"
assert_not_exists "$DNF_LOG"

reset_case
export TEST_ARCH=aarch64
run_failure bad_arch "$INSTALLER" install
assert_contains "$TEST_ROOT/bad_arch.out" "unsupported architecture"
assert_not_exists "$CURL_LOG"

reset_case
: >"$STATE_FILE"
run_success status env TMPDIR="$TEST_ROOT/nonexistent" "$INSTALLER" status
assert_contains "$TEST_ROOT/status.out" "Installed GitHub Copilot: 1.1.1-1"
assert_not_exists "$CURL_LOG"
assert_not_exists "$DNF_LOG"
assert_not_exists "$TEST_ROOT/nonexistent"

reset_case
: >"$STATE_FILE"
fake_home="$TEST_ROOT/home"
mkdir -p "$fake_home/.copilot"
printf 'keep\n' >"$fake_home/.copilot/data"
run_success uninstall env HOME="$fake_home" "$INSTALLER" uninstall --assumeyes
assert_contains "$DNF_LOG" "-y remove github"
assert_contains "$fake_home/.copilot/data" "keep"

reset_case
run_failure invalid_version "$INSTALLER" install --app-version '../bad'
assert_contains "$TEST_ROOT/invalid_version.out" "invalid application version"

reset_case
export TEST_DNF_FAIL=1
run_failure dnf_failure "$INSTALLER" install --assumeyes
assert_contains "$DNF_LOG" "-y install"

reset_case
run_success status_missing env TMPDIR="$TEST_ROOT/nonexistent" "$INSTALLER" status
assert_contains "$TEST_ROOT/status_missing.out" "not installed"
assert_not_exists "$CURL_LOG"
assert_not_exists "$DNF_LOG"

reset_case
run_failure status_version "$INSTALLER" status --app-version 1.1.1
assert_contains "$TEST_ROOT/status_version.out" "--app-version is not valid with status"
assert_not_exists "$CURL_LOG"

reset_case
run_success pinned_install "$INSTALLER" install --app-version 1.1.1 --assumeyes
assert_contains "$CURL_LOG" "/releases/tags/v1.1.1"

reset_case
run_failure mismatched_release "$INSTALLER" install --app-version 1.1.2
assert_contains "$TEST_ROOT/mismatched_release.out" "instead of 1.1.2"
assert_not_exists "$DNF_LOG"

reset_case
export TEST_CHECKSIG_FAIL=1
run_failure internal_digest "$INSTALLER" install
assert_contains "$TEST_ROOT/internal_digest.out" "internal digest checks"
assert_not_exists "$DNF_LOG"

reset_case
export TEST_CURL_FAIL=1
run_failure download_failure "$INSTALLER" install
assert_not_exists "$DNF_LOG"

reset_case
export TEST_SIGNAL=1
if "$INSTALLER" install >"$TEST_ROOT/interrupted.out" 2>&1; then
    fail "interrupted install unexpectedly succeeded"
else
    result=$?
    [[ "$result" == 143 ]] || fail "TERM should exit 143, got $result"
fi
passed=$((passed + 1))
assert_not_exists "$DNF_LOG"

if find "$TEST_ROOT" -maxdepth 1 -type d -name 'github-copilot-installer.*' |
    grep -q .; then
    fail "installer temporary directory was not cleaned"
fi

printf 'PASS: %d installer tests\n' "$passed"
