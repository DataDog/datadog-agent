#!/usr/bin/env bash
# Minimizes the kernel BTFs listed in a BTFHub bucket manifest for CO-RE
# objects or their relocation stub.
#
# usage: minimize_btfs.sh BPFTOOL MANIFEST REMOTE_FILE OUT_DIR OBJECT...
# With FULL_BTF_DIR set, the full kernel BTFs are kept there as well.
set -euo pipefail

bpftool=$(realpath "$1")
manifest=$(realpath "$2")
remote=$(<"$3")
mkdir -p "$4"
out=$(realpath "$4")
shift 4
full=${FULL_BTF_DIR:+$(mkdir -p "$FULL_BTF_DIR" && realpath "$FULL_BTF_DIR")}

# Empty objects are stubs without CO-RE relocations, which need no kernel type.
objects=()
for o; do
  [[ -s "$o" ]] && objects+=("$(realpath "$o")")
done
[[ ${#objects[@]} -gt 0 && -s "$manifest" ]] || exit 0

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
export GIT_DIR=$work/git GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null HOME=$work
git init -q --bare

mapfile -t blobs < <(cut -d' ' -f1 "$manifest")
for attempt in 1 2 3; do
  git -c fetch.negotiationAlgorithm=noop fetch -q --no-tags --no-write-fetch-head "$remote" "${blobs[@]}" && break
  [[ $attempt == 3 ]] && exit 1
  sleep $((attempt * 10))
done

while read -r blob path; do
  mkdir -p "$out/$(dirname "$path")"
  git cat-file blob "$blob" | xz -dc | tar -xOf - >"$work/kernel.btf"
  "$bpftool" gen min_core_btf "$work/kernel.btf" "$out/$path" "${objects[@]}"
  if [[ -n $full ]]; then
    mkdir -p "$full/$(dirname "$path")"
    cp "$work/kernel.btf" "$full/$path"
  fi
done <"$manifest"
