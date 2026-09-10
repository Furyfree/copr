#!/usr/bin/env bash
set -euo pipefail
mapfile -t sources < <(find cmd internal -type f -name '*.go' -print)
unformatted=$(gofmt -l "${sources[@]}")
if [[ -n "$unformatted" ]]; then
    printf 'Run gofmt on:\n%s\n' "$unformatted" >&2
    exit 1
fi
