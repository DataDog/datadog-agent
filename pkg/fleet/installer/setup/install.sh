#!/bin/bash
# Installer for Datadog (www.datadoghq.com).
# Copyright 2016-present Datadog, Inc.
#
set -euo pipefail
umask 0

os=$(uname -s)
arch=$(uname -m)

if ! { { [[ "$os" == "Linux" && ( "$arch" == "x86_64" || "$arch" == "aarch64" ) ]]; } || { [[ "$os" == "Darwin" && "$arch" == "arm64" ]]; }; }; then
  echo "This installer only supports Linux (amd64/arm64) or macOS (arm64)." >&2
  exit 1
fi

if ! command -v sha256sum >/dev/null || ! (command -v curl >/dev/null || command -v wget >/dev/null); then
  echo "This installer requires sha256sum and either curl or wget to be installed." >&2
  exit 1
fi

flavor="INSTALLER_FLAVOR"
version="INSTALLER_VERSION"
export DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_INSTALLER="$version"
case "$os" in
  Linux)
    case "$arch" in
      x86_64)
        installer_sha256="INSTALLER_AMD64_SHA256"
        ;;
      aarch64)
        installer_sha256="INSTALLER_ARM64_SHA256"
        ;;
    esac
    ;;
  Darwin)
    case "$arch" in
      arm64)
        installer_sha256="INSTALLER_DARWIN_ARM64_SHA256"
        ;;
    esac
    ;;
esac
site=${DD_SITE:-datadoghq.com}
installer_domain=${DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE:-$([[ "$site" == "datad0g.com" ]] && echo "install.datad0g.com" || echo "install.datadoghq.com")}
installer_url="https://${installer_domain}/v2/PACKAGE_NAME/blobs/sha256:${installer_sha256}"

tmp_dir="/opt/datadog-packages/tmp"
tmp_bin="${tmp_dir}/installer"

if ((UID == 0)); then
  sudo_cmd=()
  sudo_env_cmd=()
else
  sudo_cmd=(sudo)
  sudo_env_cmd=(sudo -E)
fi

# This migrates legacy installs by removing the legacy deb / rpm installer package
if command -v dpkg >/dev/null && dpkg -s datadog-installer >/dev/null 2>&1; then
  "${sudo_cmd[@]+"${sudo_cmd[@]}"}" datadog-installer purge >/dev/null 2>&1 || true
  "${sudo_cmd[@]+"${sudo_cmd[@]}"}" dpkg --purge datadog-installer >/dev/null 2>&1 || true
elif command -v rpm >/dev/null && rpm -q datadog-installer >/dev/null 2>&1; then
  "${sudo_cmd[@]+"${sudo_cmd[@]}"}" datadog-installer purge >/dev/null 2>&1 || true
  "${sudo_cmd[@]+"${sudo_cmd[@]}"}" rpm -e datadog-installer >/dev/null 2>&1 || true
fi

"${sudo_cmd[@]+"${sudo_cmd[@]}"}" mkdir -p "$tmp_dir"

echo "Downloading the Datadog installer..."
if command -v curl >/dev/null; then
  if ! curl -L --retry 3 "$installer_url" | "${sudo_cmd[@]+"${sudo_cmd[@]}"}" tee "$tmp_bin" >/dev/null; then
    echo "Error: Download failed with curl." >&2
    exit 1
  fi
else
  if ! wget --tries=3 -O - "$installer_url" | "${sudo_cmd[@]+"${sudo_cmd[@]}"}" tee "$tmp_bin" >/dev/null; then
    echo "Error: Download failed with wget." >&2
    exit 1
  fi
fi
"${sudo_cmd[@]+"${sudo_cmd[@]}"}" chmod +x "$tmp_bin"

echo "Verifying installer integrity..."
actual_sha256=$(shasum -a 256 "$tmp_bin" | awk '{print $1}')
if [ "$installer_sha256" != "$actual_sha256" ]; then
  echo "Checksum mismatch!"
  exit 1
fi
echo "Installer integrity verified."

echo "Starting the Datadog installer..."
"${sudo_env_cmd[@]+"${sudo_env_cmd[@]}"}" "$tmp_bin" setup --flavor "$flavor" "$@"

"${sudo_cmd[@]+"${sudo_cmd[@]}"}" rm -f "$tmp_bin"

exit 0
