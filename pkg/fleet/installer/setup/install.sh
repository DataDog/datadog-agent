#!/bin/bash
set -e

os=$(uname -s)
arch=$(uname -m)

if [[ "$os" != "Linux" || ("$arch" != "x86_64" && "$arch" != "aarch64") ]]; then
  echo "This installer only supports Linux running on amd64 or arm64." >&2
  exit 1
fi

if ! command -v sha256sum >/dev/null || ! (command -v curl >/dev/null || command -v wget >/dev/null); then
  echo "This installer requires sha256sum and either curl or wget to be installed." >&2
  exit 1
fi

case "$arch" in
x86_64)
  installer_url="LINUX_INSTALLER_URL_AMD64"
  installer_sha256="LINUX_INSTALLER_URL_AMD64_SHA256"
  ;;
aarch64)
  installer_url="LINUX_INSTALLER_URL_ARM64"
  installer_sha256="LINUX_INSTALLER_URL_ARM64_SHA256"
  ;;
esac

tmp_dir="/opt/datadog-packages/tmp"
tmp_bin="${tmp_dir}/installer"

if ((UID == 0)); then
  sudo_cmd=()
  sudo_env_cmd=()
else
  sudo_cmd=(sudo)
  sudo_env_cmd=(sudo -E)
fi

"${sudo_cmd[@]}" mkdir -p "$tmp_dir"

echo "Downloading the Datadog installer..."
if command -v curl >/dev/null; then
  curl -L --retry 3 -o "$tmp_bin" "$installer_url"
else
  wget --tries=3 -O "$tmp_bin" "$installer_url"
fi

echo "Verifying installer integrity..."
sha256sum -c <<<"$installer_sha256  $tmp_bin"

echo "Starting the Datadog installer..."
"${sudo_cmd[@]}" chmod +x "$tmp_bin"
"${sudo_env_cmd[@]}" "$tmp_bin" "$@"

"${sudo_cmd[@]}" rm -f "$tmp_bin"

exit 0
