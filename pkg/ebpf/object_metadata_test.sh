#!/usr/bin/env bash
# Usage: object_metadata_test.sh <readelf> <object>...
# Every eBPF object must include bpf_metadata.h, which embeds <key:value>
# entries in the dd_metadata section.
set -euo pipefail

readelf="$1"
shift

failed=0
for obj in "$@"; do
    out="$("$readelf" -p dd_metadata "$obj" 2>&1)" || true
    if ! grep -qE '<[^:>]+:[^>]+>' <<<"$out"; then
        echo "$obj: missing dd_metadata, include bpf_metadata.h" >&2
        failed=1
    fi
done

[[ "$failed" == 0 ]] && echo "All $# object files have valid metadata"
exit "$failed"
