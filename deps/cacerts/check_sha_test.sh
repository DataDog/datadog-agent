#!/bin/bash

CERTS_DIR="${TEST_SRCDIR}/_main/deps/cacerts"

cd "${CERTS_DIR}"
sha256sum --check cacert.sha256
