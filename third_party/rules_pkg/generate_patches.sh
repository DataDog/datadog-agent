#!/usr/bin/env bash
# Generate patch files for third_party/rules_pkg.
#
# Downloads the upstream source file at the exact commit pinned in MODULE.bazel,
# diffs it against our modified version, and writes the result to patches/.
#
# Usage (from repo root):
#   bash third_party/rules_pkg/generate_patches.sh
#
# When updating the upstream commit in MODULE.bazel:
#   1. Update COMMIT below to match the new commit.
#   2. Update the modified source file(s) in third_party/rules_pkg/ to reflect
#      any upstream changes while preserving our modifications.
#   3. Re-run this script to regenerate the patch(es).
#   4. Commit the updated source and patch files together.

set -euo pipefail

# Must match the commit in the rules_pkg git_override() in MODULE.bazel.
COMMIT="401969d4367c42dcbb45d33a637eae87788d025e"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH_DIR="${SCRIPT_DIR}/patches"

generate_patch() {
    local upstream_path="$1"   # path within the rules_pkg repo, e.g. pkg/private/tar/tar.bzl
    local local_file="$2"      # path to our modified version (absolute)
    local patch_name="$3"      # output patch filename (no directory)

    local upstream_url="https://raw.githubusercontent.com/bazelbuild/rules_pkg/${COMMIT}/${upstream_path}"
    local tmp
    tmp="$(mktemp)"
    trap "rm -f '${tmp}'" EXIT

    echo "Downloading ${upstream_url} ..."
    curl -fsSL "${upstream_url}" -o "${tmp}"

    echo "Generating ${patch_name} ..."
    # diff exits 1 when files differ (which is expected here).
    diff -u \
        --label "a/${upstream_path}" \
        --label "b/${upstream_path}" \
        "${tmp}" \
        "${local_file}" \
        > "${PATCH_DIR}/${patch_name}" || true

    if [ ! -s "${PATCH_DIR}/${patch_name}" ]; then
        echo "WARNING: patch is empty — local file matches upstream exactly."
    else
        echo "  -> ${PATCH_DIR}/${patch_name}"
    fi
}

generate_patch \
    "pkg/private/tar/tar.bzl" \
    "${SCRIPT_DIR}/pkg/private/tar/tar.bzl" \
    "tar_bzl.patch"

echo "Done."
