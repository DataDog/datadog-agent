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
    sudo_cmd_with_envs=''
  else
    sudo_cmd='sudo'
    sudo_cmd_with_envs='sudo -E'
  fi

  $sudo_cmd mkdir -p "${tmp_dir}"
  case "$(uname -m)" in
  x86_64)
    write_installer_amd64 "$sudo_cmd" "$downloader_path"
    ;;
  aarch64)
    write_installer_arm64 "$sudo_cmd" "$downloader_path"
    ;;
  esac
  $sudo_cmd chmod +x "${downloader_path}"
  echo "Starting the Datadog installer..."
  $sudo_cmd_with_envs "${downloader_path}" "$@"
  $sudo_cmd rm -f "${downloader_path}"
}

# Embedded binaries used to install Datadog.
# Source: https://github.com/DataDog/datadog-agent/tree/INSTALLER_COMMIT/pkg/fleet/installer
# DO NOT EDIT THIS SECTION MANUALLY.
write_installer_amd64() {
  local sudo_cmd=$1
  local path=$2
  base64 -d <<<"DOWNLOADER_BIN_LINUX_AMD64" | $sudo_cmd tee "${path}" >/dev/null
}
write_installer_arm64() {
  local sudo_cmd=$1
  local path=$2
  base64 -d <<<"DOWNLOADER_BIN_LINUX_ARM64" | $sudo_cmd tee "${path}" >/dev/null
}

install "$@"
exit 0
