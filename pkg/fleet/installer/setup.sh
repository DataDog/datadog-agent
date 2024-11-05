#!/bin/sh
set -e
# This script is used to install Datadog.
# Note to developers: This script is only responsible for starting the actual installer binary.
# Keep the installation logic in the installer binary itself.

if [ "$(uname -s)" != "Linux" ] || { [ "$(uname -m)" != "x86_64" ] && [ "$(uname -m)" != "aarch64" ]; }; then
  echo "This installer only supports linux running on amd64 or arm64." >&2
  exit 1
fi

installer_path="datadog-installer"
keyring_path="datadog-keyring.gpg"

install() {
  case "$(uname -m)" in
  x86_64)
    echo "${installer_bin_linux_amd64}" | base64 -d >"${installer_path}"
    echo "${installer_sig_linux_amd64}" | base64 -d >"${installer_path}.sig"
    ;;
  aarch64)
    echo "${installer_bin_linux_arm64}" | base64 -d >"${installer_path}"
    echo "${installer_sig_linux_arm64}" | base64 -d >"${installer_path}.sig"
    ;;
  esac
  verify
  chmod +x "${installer_path}"
  echo "Running the installer binary..."
  ./"${installer_path}" "$@"
  rm -f "${installer_path}" "${installer_path}.sig"
}

verify() {
  if [ ! -s "${installer_path}.sig" ]; then
    echo "Warning: Signature file is empty. Skipping signature verification." >&2
    return
  fi
  if ! command -v gpgv >/dev/null || ! command -v curl >/dev/null; then
    echo "Warning: gpgv or curl are not installed. Skipping signature verification." >&2
    return
  fi

  echo "Verifying the installer binary with the signature..."
  curl -sSL "https://install.datadoghq.com/DATADOG_KEY_CURRENT.gpg" | gpgv --import --batch --no-default-keyring --keyring "${keyring_path}"
  if ! gpgv --verify "${installer_path}.sig" "${installer_path}" 2>/dev/null; then
    echo "Error: Signature verification failed." >&2
    exit 1
  fi
  rm -f "${keyring_path}"
  echo "Signature verification successful."
}

# Embedded installer binaries.
# Source: https://github.com/DataDog/datadog-agent/blob/main/cmd/installer
installer_bin_linux_amd64="INSTALLER_BIN_LINUX_AMD64"
installer_sig_linux_amd64="INSTALLER_SIG_LINUX_AMD64"
installer_bin_linux_arm64="INSTALLER_BIN_LINUX_ARM64"
installer_sig_linux_arm64="INSTALLER_SIG_LINUX_ARM64"

install "$@"
