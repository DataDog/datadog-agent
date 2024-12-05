#!/bin/bash
set -e
# This script is used to install Datadog.

if [ "$(uname -s)" != "Linux" ] || { [ "$(uname -m)" != "x86_64" ] && [ "$(uname -m)" != "aarch64" ]; }; then
  echo "This installer only supports linux running on amd64 or arm64." >&2
  exit 1
fi

installer_path="/opt/datadog-installer-bootstrap"

install() {
  if [ "$UID" == "0" ]; then
    sudo_cmd=''
  else
    sudo_cmd='sudo'
  fi

  case "$(uname -m)" in
  x86_64)
    echo "${installer_bin_linux_amd64}" | base64 -d | $sudo_cmd tee "${installer_path}" >/dev/null
    ;;
  aarch64)
    echo "${installer_bin_linux_arm64}" | base64 -d | $sudo_cmd tee "${installer_path}" >/dev/null
    ;;
  esac
  $sudo_cmd chmod +x "${installer_path}"
  echo "Running the installer binary..."
  $sudo_cmd "${installer_path}" "$@"
  $sudo_cmd rm -f "${installer_path}"
}

# Embedded installer binaries.
# Source: https://github.com/DataDog/datadog-agent/tree/INSTALLER_COMMIT/cmd/installer
installer_bin_linux_amd64=$(
  cat <<EOM
INSTALLER_BIN_LINUX_AMD64
EOM
)
installer_bin_linux_arm64=$(
  cat <<EOM
INSTALLER_BIN_LINUX_ARM64
EOM
)

install "$@"
exit 0
