#!/bin/sh
set -e
# This script is used to install the Datadog.
# Note to developer: This script is only responsible of starting the actual installer binary.

if [ "$(uname -s)" != "Linux" ] || { [ "$(uname -m)" != "x86_64" ] && [ "$(uname -m)" != "aarch64" ]; }; then
  echo "This installer only supports linux running on amd64 or arm64." >&2
  exit 1
fi

installer_path="datadog-installer"

install() {
  case "$(uname -m)" in
  x86_64)
    echo "${installer_bin_linux_amd64}" | base64 -d >"${installer_path}"
    ;;
  aarch64)
    echo "${installer_bin_linux_arm64}" | base64 -d >"${installer_path}"
    ;;
  esac
  chmod +x "${installer_path}"
  ./"${installer_path}" "$@"
}

# Embedded installer binaries.
# Source: https://github.com/DataDog/datadog-agent/blob/main/cmd/installer
installer_bin_linux_amd64="INSTALLER_BIN_LINUX_AMD64"
installer_bin_linux_arm64="INSTALLER_BIN_LINUX_ARM64"

install "$@"
