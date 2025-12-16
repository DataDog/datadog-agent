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
    command_on_repo_root "bazelisk run -- @openssl_fips//:install --destdir=#{install_dir}"
    
    # Calling helpers to set the correct paths in openssl.cnf and fipsinstall.sh.
    if windows?
      # We pass both arguments as a single string to avoid powershell parsing issues.
      command_on_repo_root "bazelisk run -- @openssl_fips//:configure_fips --destdir=\"#{install_dir}\" --embedded_ssl_dir=\"C:/Program Files/Datadog/Datadog Agent/embedded3/ssl\""
    else
      command_on_repo_root "bazelisk run -- @openssl_fips//:configure_fips --destdir=#{install_dir}"
      command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix #{install_dir}/embedded" \
        " #{install_dir}/embedded/lib/ossl-modules/fips.so" \
    end
end
