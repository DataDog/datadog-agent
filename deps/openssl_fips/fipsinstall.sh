#!/bin/bash

# Run the OpenSSL FIPS self-tests and generate fipsmodule.cnf for this machine.
# The OpenSSL security policy requires the self-tests to be run and the config
# file to be generated locally — it cannot be copied between machines.
#
# openssl.cnf is shipped as openssl.cnf.tmp because it references fipsmodule.cnf
# which doesn't exist yet. This script generates fipsmodule.cnf, then moves
# openssl.cnf.tmp → openssl.cnf and rewrites its .include line to the physical
# path of fipsmodule.cnf so it remains valid if the tree is relocated (OCI
# installs use a per-version directory; the build-time path would be wrong).

set -euo pipefail

# Resolve the physical path so .include in openssl.cnf points at the actual
# directory, not a symlink that may be retargeted later (e.g. experiment/ → stable/).
INSTALL_DIR="$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")"

FIPS_MODULE_PATH="${INSTALL_DIR}/ssl/fipsmodule.cnf"
OPENSSL_CONF_PATH="${INSTALL_DIR}/ssl/openssl.cnf"
FIPS_SO_PATH="${INSTALL_DIR}/lib/ossl-modules/fips.so"
OPENSSL_BIN="${INSTALL_DIR}/bin/openssl"

# Regenerate fipsmodule.cnf (remove stale copy if present).
rm -f "${FIPS_MODULE_PATH}"
"${OPENSSL_BIN}" fipsinstall -module "${FIPS_SO_PATH}" -out "${FIPS_MODULE_PATH}"

# Activate openssl.cnf and fix its .include to the physical fipsmodule.cnf path.
mv "${OPENSSL_CONF_PATH}.tmp" "${OPENSSL_CONF_PATH}"
sed -i "s#^\.include .*/fipsmodule\.cnf#.include ${FIPS_MODULE_PATH}#" "${OPENSSL_CONF_PATH}"
if ! grep -qF ".include ${FIPS_MODULE_PATH}" "${OPENSSL_CONF_PATH}"; then
    echo "fipsinstall: failed to update .include path in ${OPENSSL_CONF_PATH}"
    exit 1
fi

# Verify the module is correctly installed.
if ! "${OPENSSL_BIN}" fipsinstall -module "${FIPS_SO_PATH}" -in "${FIPS_MODULE_PATH}" -verify; then
    echo "fipsinstall: verification failed — ${FIPS_MODULE_PATH} may be corrupted"
    exit 1
fi
