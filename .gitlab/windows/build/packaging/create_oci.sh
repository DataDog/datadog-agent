#!/usr/bin/env bash

set -euo pipefail
shopt -s nullglob

PIPE_DIR="$OMNIBUS_PACKAGE_DIR/pipeline-$CI_PIPELINE_ID"
mapfile -t ARTIFACTS < <(compgen -G "${PIPE_DIR}/${WIN_ARTIFACT_PATTERN}")
if [ "${#ARTIFACTS[@]}" -eq 0 ]; then
    echo "No suitable input artifact found"
    exit 1
fi
if [ "${#ARTIFACTS[@]}" -ne 1 ]; then
    echo "Expected exactly one candidate input artifact"
    exit 1
fi
if [ -z "${PACKAGE_VERSION:-}" ]; then
    echo "PACKAGE_VERSION is not set"
    exit 1
fi
ARTIFACT="${ARTIFACTS[0]}"

SRC_DIR="$(mktemp -d)"
EXTRA_FLAGS=()
case "${WIN_SOURCE_TYPE}" in
    msi)
        if [ -n "${WIN_MSI_DESTINATION_PRODUCT:-}" ]; then
            cp "${ARTIFACT}" "${SRC_DIR}/${WIN_MSI_DESTINATION_PRODUCT}-${PACKAGE_VERSION}-x86_64.msi"
        else
            cp "${ARTIFACT}" "${SRC_DIR}/"
        fi
        ;;
    zip)
        unzip -q "${ARTIFACT}" -d "${SRC_DIR}"
        if [ -d "${SRC_DIR}/etc/datadog-agent" ]; then
            EXTRA_FLAGS+=(--configs "${SRC_DIR}/etc/datadog-agent")
        fi
        ;;
    *)
        echo "Unknown WIN_SOURCE_TYPE: ${WIN_SOURCE_TYPE}"
        exit 1
        ;;
esac

# Add installer binary for datadog-agent OCI package
if [ "${OCI_PRODUCT}" = "datadog-agent" ]; then
    INSTALLER_BINS=("${PIPE_DIR}"/datadog-installer-*-x86_64.exe)
    if [ "${#INSTALLER_BINS[@]}" -eq 0 ]; then
        echo "ERROR: No installer binary found for datadog-agent OCI package"
        exit 1
    fi
    EXTRA_FLAGS+=(--installer "${INSTALLER_BINS[0]}")
fi

PATH="$PATH:$(go env GOPATH)/bin"
export PATH
VARIANT_ARGS=()
ARCHIVE_VARIANT=""
if [ -n "${OCI_VARIANT:-}" ]; then
    VARIANT_ARGS=(--variant "${OCI_VARIANT}")
    ARCHIVE_VARIANT="-${OCI_VARIANT}"
fi

datadog-package create \
    --version "${PACKAGE_VERSION}" \
    --package "${OCI_PRODUCT}" \
    --os windows \
    --arch amd64 \
    "${VARIANT_ARGS[@]}" \
    --archive \
    --archive-path "${PIPE_DIR}/${OCI_PRODUCT}-${PACKAGE_VERSION}-windows-amd64${ARCHIVE_VARIANT}.oci.tar" \
    "${EXTRA_FLAGS[@]}" \
    "${SRC_DIR}/"

ls -l "${PIPE_DIR}"
