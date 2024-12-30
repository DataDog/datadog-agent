# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com).
# Copyright 2016-present Datadog, Inc.

# Embedded OpenSSL to meet FIPS requirements. It comes in two parts:
# 1. The FIPS module itself (this software definition). It must use a FIPS-validated version
#    and follow the build steps outlined in the OpenSSL FIPS Security Policy.
# 2. The OpenSSL library, which can be any 3.0.x version. This library will use the FIPS provider.

name "openssl-fips-provider"
default_version "0.0.1"

OPENSSL_FIPS_MODULE_VERSION="3.0.9"
OPENSSL_FIPS_MODULE_FILENAME="openssl-#{OPENSSL_FIPS_MODULE_VERSION}.tar.gz"
OPENSSL_FIPS_MODULE_SHA256_SUM="eb1ab04781474360f77c318ab89d8c5a03abc38e63d65a603cabbf1b00a1dc90"

source url: "https://www.openssl.org/source/#{OPENSSL_FIPS_MODULE_FILENAME}",
        sha256: "#{OPENSSL_FIPS_MODULE_SHA256_SUM}",
        extract: :seven_zip

relative_path "openssl-#{OPENSSL_FIPS_MODULE_VERSION}"

build do
    env = with_standard_compiler_flags()
    env["MAKEFLAGS"] = "-j#{workers}"
    # Exact build steps from security policy:
    # https://csrc.nist.gov/CSRC/media/projects/cryptographic-module-validation-program/documents/security-policies/140sp4282.pdf
    #
    # ---------------- DO NOT MODIFY LINES BELOW HERE ----------------
    command "./Configure enable-fips", env: env

    command "make", env: env
    command "make install", env: env
    # ---------------- DO NOT MODIFY LINES ABOVE HERE ----------------

    mkdir "#{install_dir}/embedded/ssl"
    mkdir "#{install_dir}/embedded/lib/ossl-modules"
    copy "/usr/local/lib*/ossl-modules/fips.so", "#{install_dir}/embedded/lib/ossl-modules/fips.so"

    erb source: "openssl.cnf.erb",
        dest: "#{install_dir}/embedded/ssl/openssl.cnf.tmp",
        mode: 0644,
        vars: { install_dir: install_dir }
    erb source: "fipsinstall.sh.erb",
        dest: "#{install_dir}/embedded/bin/fipsinstall.sh",
        mode: 0755,
        vars: { install_dir: install_dir }
end
