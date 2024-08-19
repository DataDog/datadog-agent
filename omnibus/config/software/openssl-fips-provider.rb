# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Embedded OpenSSL to meet FIPS requirements. It comes in two parts:
# 1. The FIPS module itself (this software definition). It must use a FIPS-validated version 
#    and follow the build steps outlined in the OpenSSL FIPS Security Policy.
# 2. The OpenSSL library, which can be any 3.0.x version. This library will use the FIPS provider. 

name "openssl-fips-provider"
default_version "0.0.1"

OPENSSL_FIPS_MODULE_VERSION="3.0.8"
OPENSSL_FIPS_MODULE_FILENAME="openssl-#{OPENSSL_FIPS_MODULE_VERSION}.tar.gz"
OPENSSL_FIPS_MODULE_SHA256_SUM="6c13d2bf38fdf31eac3ce2a347073673f5d63263398f1f69d0df4a41253e4b3e"

source url: "https://www.openssl.org/source/#{OPENSSL_FIPS_MODULE_FILENAME}",
        sha256: "#{OPENSSL_FIPS_MODULE_SHA256_SUM}",
        extract: :seven_zip,
        target_filename: "#{OPENSSL_FIPS_MODULE_FILENAME}"

relative_path "openssl-#{OPENSSL_FIPS_MODULE_VERSION}"

build do
    # Exact build steps from security policy:
    # https://csrc.nist.gov/CSRC/media/projects/cryptographic-module-validation-program/documents/security-policies/140sp4282.pdf
    #
    # ---------------- DO NOT MODIFY LINES BELOW HERE ----------------
    command "./Configure enable-fips"

    command "make"
    command "make install"
    # ---------------- DO NOT MODIFY LINES ABOVE HERE ----------------
end 