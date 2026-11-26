#!/usr/bin/env bash
# Checks that minimizing kernel BTFs for a CO-RE relocation stub gives the
# same BTFs as minimizing them for the CO-RE objects themselves, and that
# cilium/ebpf relocates the objects against them as against the full BTFs.
#
# usage: minimized_btfs_test.sh MINIMIZE BPFTOOL CORECHECK REMOTE_FILE KERNELS STUB MANIFEST... -- OBJECT...
# KERNELS is a comma-separated list of output paths from the manifests.
set -euo pipefail

minimize=$1 bpftool=$2 corecheck=$3 remote=$4 kernels=$5 stub=$6
shift 6
manifests=()
while [[ $1 != -- ]]; do
  manifests+=("$1")
  shift
done
shift

tmp=${TEST_TMPDIR:-$(mktemp -d)}
tr , '\n' <<<"$kernels" >"$tmp/kernels"
awk 'NR == FNR { want[$1]; next } $2 in want' "$tmp/kernels" "${manifests[@]}" >"$tmp/manifest"
if [[ $(wc -l <"$tmp/manifest") != $(wc -l <"$tmp/kernels") ]]; then
  echo "kernels missing from the BTFHub archive:"
  cut -d' ' -f2 "$tmp/manifest" | grep -vxF -f - "$tmp/kernels"
  exit 1
fi

"$minimize" "$bpftool" "$tmp/manifest" "$remote" "$tmp/stub" "$stub"
FULL_BTF_DIR=$tmp/full "$minimize" "$bpftool" "$tmp/manifest" "$remote" "$tmp/objects" "$@"
diff -r "$tmp/objects" "$tmp/stub"
echo "$(find "$tmp/stub" -type f | wc -l) minimized BTFs are identical"
"$corecheck" "$tmp/full" "$tmp/stub" "$@"
