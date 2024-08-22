#!/bin/bash

# The OpenSSL security policy states:
# "The Module shall have the self-tests run, and the Module config file output generated on each
# platform where it is intended to be used. The Module config file output data shall not be copied from
# one machine to another."
# This script aims to run self-tests and generate `fipsmodule.cnf.`
# Because the provided `openssl.cnf` references to `fipsmodule.cnf` which is not yet created, we first create it
# as `openssl.cnf.tmp` and then move it to its final name `openssl.cnf` when `fipsmodule.cnf` has been created

# TODO: template INSTALL_DIR fipsinstall.sh.erb
set -euo pipefail

INSTALL_DIR="/opt/datadog-agent/embedded"

FIPS_MODULE_PATH="${INSTALL_DIR}/ssl/fipsmodule.cnf"
OPENSSL_CONF_PATH="${INSTALL_DIR}/ssl/openssl.cnf"

FIPS_SO_PATH="${INSTALL_DIR}/lib/ossl-modules/fips.so"
OPENSSL_BIN="${INSTALL_DIR}/bin/openssl"


if [ ! -f "${FIPS_MODULE_PATH}" ]; then
    "${OPENSSL_BIN}" fipsinstall -module "${FIPS_SO_PATH}" -out "${FIPS_MODULE_PATH}"
    mv "${OPENSSL_CONF_PATH}.tmp" "${OPENSSL_CONF_PATH}"
fi

if ! "${OPENSSL_BIN}" fipsinstall -module "${FIPS_SO_PATH}" -in "${FIPS_MODULE_PATH}" -verify; then
    echo "openssl fipsinstall: verification of FIPS compliance failed. $INSTALL_DIR/fipsmodule.cnf was corrupted or the installation failed."
fi