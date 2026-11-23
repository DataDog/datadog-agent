#!/usr/bin/env bash
# Checks that minimizing kernel BTFs for a CO-RE relocation stub gives the
# same BTFs as minimizing them for the CO-RE objects themselves.
#
# usage: minimized_btfs_test.sh MINIMIZE BPFTOOL REMOTE_FILE KERNELS STUB MANIFEST... -- OBJECT...
# KERNELS is a comma-separated list of output paths from the manifests.
set -euo pipefail

minimize=$1 bpftool=$2 remote=$3 kernels=$4 stub=$5
shift 5
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
"$minimize" "$bpftool" "$tmp/manifest" "$remote" "$tmp/objects" "$@"
diff -r "$tmp/objects" "$tmp/stub"
echo "$(find "$tmp/stub" -type f | wc -l) minimized BTFs are identical"
