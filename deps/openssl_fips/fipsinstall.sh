#!/bin/bash

# The OpenSSL security policy states:
# "The Module shall have the self-tests run, and the Module config file output generated on each
# platform where it is intended to be used. The Module config file output data shall not be copied from
# one machine to another."
# This script aims to run self-tests and generate `fipsmodule.cnf.`
# Because the provided `openssl.cnf` references to `fipsmodule.cnf` which is not yet created, we first create it
# as `openssl.cnf.tmp` and then move it to its final name `openssl.cnf` when `fipsmodule.cnf` has been created

set -euo pipefail

# INSTALL_DIR is the `embedded` directory of the package tree. Derive it from
# the script's own location (embedded/bin/fipsinstall.sh) rather than baking an
# absolute path at build time: the OCI installer flow extracts/moves the tree to
# a per-version path (and to temporary staging paths) that does not match the
# build-time location, so a hardcoded path would run fipsinstall against the
# wrong tree or fail outright. Resolving relative to the script keeps the
# self-test and the generated fipsmodule.cnf pinned to the tree it belongs to.
INSTALL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

FIPS_MODULE_PATH="${INSTALL_DIR}/ssl/fipsmodule.cnf"
OPENSSL_CONF_PATH="${INSTALL_DIR}/ssl/openssl.cnf"

FIPS_SO_PATH="${INSTALL_DIR}/lib/ossl-modules/fips.so"
OPENSSL_BIN="${INSTALL_DIR}/bin/openssl"


if [ -f "${FIPS_MODULE_PATH}" ]; then
    rm "${FIPS_MODULE_PATH}"
fi

"${OPENSSL_BIN}" fipsinstall -module "${FIPS_SO_PATH}" -out "${FIPS_MODULE_PATH}"
mv "${OPENSSL_CONF_PATH}.tmp" "${OPENSSL_CONF_PATH}"

# Point the config's fipsmodule.cnf include at this tree's actual location. The
# build bakes an absolute path (via {{embedded_ssl_dir}}) that is wrong once the
# tree is relocated to a per-version OCI path, and OpenSSL resolves a relative
# .include against the current working directory (not the config file), so
# neither the baked path nor a relative include is safe. Rewrite it here, where
# we know the real on-disk location.
sed -i "s#^\.include .*/fipsmodule\.cnf#.include ${FIPS_MODULE_PATH}#" "${OPENSSL_CONF_PATH}"

if ! "${OPENSSL_BIN}" fipsinstall -module "${FIPS_SO_PATH}" -in "${FIPS_MODULE_PATH}" -verify; then
    echo "openssl fipsinstall: verification of FIPS compliance failed. $INSTALL_DIR/fipsmodule.cnf was corrupted or the installation failed."
fi
