#!/usr/bin/env bash
# Asserts that the whole_distro_tar_deb tarball contains the expected set of
# entries under /opt/datadog-installer. This replaces a manual "tar tvf"
# sanity check that was previously run (and merely printed) from
# omnibus/config/software/installer.rb.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <path-to-tar>" >&2
  exit 2
fi

tar_path="$1"
listing="$(tar tvf "${tar_path}")"

assert_contains() {
  local pattern="$1"
  if ! grep -qE "${pattern}" <<<"${listing}"; then
    echo "FAIL: expected tar '${tar_path}' to contain an entry matching: ${pattern}" >&2
    echo "--- actual tar contents ---" >&2
    echo "${listing}" >&2
    exit 1
  fi
}

assert_contains 'opt/datadog-installer/bin/installer/installer$'
assert_contains 'opt/datadog-installer/embedded/bin/?$'
assert_contains 'opt/datadog-installer/embedded/lib/?$'
assert_contains 'opt/datadog-installer/version-manifest\.json$'
assert_contains 'opt/datadog-installer/version-manifest\.txt$'
assert_contains 'opt/datadog-installer/README\.md$'

echo "OK: whole_distro_tar_deb contents verified"
