#!/bin/bash
# Runtime-installs the `authanywhere` binary used to mint short-lived infra tokens.
#
# The 6.53.x deb_x64 build image does not ship `authanywhere` (only dd-octo-sts),
# so jobs that need a dynamic GitLab token install it at runtime. This is the
# accepted agent6 workaround until the binary is baked into the build image.
#
# Idempotent: no-op if authanywhere is already on PATH.
set -euo pipefail

if command -v authanywhere >/dev/null 2>&1; then
    exit 0
fi

AUTHANYWHERE_VERSION="${AUTHANYWHERE_VERSION:-v0.0.2}"
AUTHANYWHERE_SHA256="${AUTHANYWHERE_SHA256:-05d14b25e4607cc9e14867f2a0f38774869ae609eec890168e57de9a1b428e37}"

case "$(uname -m)" in
    x86_64) arch="amd64" ;;
    aarch64) arch="arm64" ;;
    *) echo "Unsupported architecture for authanywhere: $(uname -m)" >&2; exit 1 ;;
esac

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

url="https://binaries.ddbuild.io/dd-source/authanywhere/${AUTHANYWHERE_VERSION}/authanywhere-tar.tar.gz"
"$(dirname "$0")/retry.sh" curl -fsSL "$url" -o "$tmpdir/authanywhere-tar.tar.gz"

echo "${AUTHANYWHERE_SHA256}  $tmpdir/authanywhere-tar.tar.gz" | sha256sum --check

tar -xf "$tmpdir/authanywhere-tar.tar.gz" -C "$tmpdir" "authanywhere-linux-${arch}"
mv "$tmpdir/authanywhere-linux-${arch}" /usr/local/bin/authanywhere
chmod +x /usr/local/bin/authanywhere
authanywhere --help >/dev/null 2>&1 || true
echo "authanywhere ${AUTHANYWHERE_VERSION} installed"
