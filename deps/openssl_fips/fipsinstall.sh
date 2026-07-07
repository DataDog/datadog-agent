#!/bin/bash

# The OpenSSL security policy states:
# "The Module shall have the self-tests run, and the Module config file output generated on each
# platform where it is intended to be used. The Module config file output data shall not be copied from
# one machine to another."
# This script aims to run self-tests and generate `fipsmodule.cnf`.
# Because the provided `openssl.cnf` references `fipsmodule.cnf` which is not yet created, we first create it
# then move `openssl.cnf.tmp` to its final name `openssl.cnf`. The .include path is also rewritten to the
# physical on-disk location so it remains valid if the tree is relocated (OCI installs place the tree at a
# per-version path that differs from the build-time path).

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
