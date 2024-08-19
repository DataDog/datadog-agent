# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Embedded OpenSSL to meet FIPS requirements. It comes in two parts:
# 1. The FIPS module itself . It must use a FIPS-validated version 
#    and follow the build steps outlined in the OpenSSL FIPS Security Policy.
# 2. The OpenSSL library (this software definition), which can be any 3.0.x version. This library will use the FIPS provider. 

name "openssl-fips"
default_version "0.0.1"

resources_path="#{Omnibus::Config.project_root}/resources/fips"

OPENSSL_VERSION="3.0.14"
OPENSSL_SHA256_SUM="eeca035d4dd4e84fc25846d952da6297484afa0650a6f84c682e39df3a4123ca"
OPENSSL_FILENAME="openssl-#{OPENSSL_VERSION}.tar.gz"

DIST_DIR="#{install_dir}/embedded"

source url: "https://www.openssl.org/source/#{OPENSSL_FILENAME}",
           sha256: "#{OPENSSL_SHA256_SUM}",
           extract: :seven_zip,
           target_filename: "#{OPENSSL_FILENAME}"

relative_path "openssl-#{OPENSSL_VERSION}"

dependency "openssl-fips-provider"

build do
    command "./Configure --prefix=\"#{DIST_DIR}\" \
                --libdir=lib \
                -Wl,-rpath=\"#{DIST_DIR}/lib\" \
                no-asm no-ssl2 no-ssl3 \
                shared zlib"

    command "make depend -j"
    command "make -j"
    command "make install_sw -j"
    command "openssl version -v"

    mkdir "#{install_dir}/embedded/ssl"
    mkdir "#{install_dir}/embedded/lib/ossl-modules"
    copy "/usr/local/lib*/ossl-modules/fips.so", "#{install_dir}/embedded/lib/ossl-modules/fips.so"

    copy "#{resources_path}/openssl.cnf", "#{install_dir}/embedded/ssl/openssl.cnf.tmp"
    copy "#{resources_path}/fipsinstall.sh", "#{install_dir}/embedded/bin/fipsinstall.sh"
end 