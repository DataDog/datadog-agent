#!/bin/bash
# Installer for Datadog (www.datadoghq.com).
# Copyright 2016-present Datadog, Inc.
#
set -e

if [ "$(uname -s)" != "Linux" ] || { [ "$(uname -m)" != "x86_64" ] && [ "$(uname -m)" != "aarch64" ]; }; then
  echo "This installer only supports linux running on amd64 or arm64." >&2
  exit 1
fi

tmp_dir="/opt/datadog-packages/tmp"
downloader_path="${tmp_dir}/download-installer"

install() {
  if [ "$UID" == "0" ]; then
    sudo_cmd=''
  else
    sudo_cmd='sudo'
  fi

  $sudo_cmd mkdir -p "${tmp_dir}"
  case "$(uname -m)" in
  x86_64)
    echo "${downloader_bin_linux_amd64}" | base64 -d | $sudo_cmd tee "${downloader_path}" >/dev/null
    ;;
  aarch64)
    echo "${downloader_bin_linux_arm64}" | base64 -d | $sudo_cmd tee "${downloader_path}" >/dev/null
    ;;
  esac
  $sudo_cmd chmod +x "${downloader_path}"
  echo "Starting the Datadog installer..."
  $sudo_cmd "${downloader_path}" "$@"
  $sudo_cmd rm -f "${downloader_path}"
}

# Embedded binaries used to install Datadog.
# Source: https://github.com/DataDog/datadog-agent/tree/INSTALLER_COMMIT/pkg/fleet/installer
# DO NOT EDIT THIS SECTION MANUALLY.
downloader_bin_linux_amd64=$(
  cat <<EOM
DOWNLOADER_BIN_LINUX_AMD64
EOM
)
downloader_bin_linux_arm64=$(
  cat <<EOM
DOWNLOADER_BIN_LINUX_ARM64
EOM
)

install "$@"
exit 0
