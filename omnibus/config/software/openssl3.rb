#
# Copyright 2023 Chef Software, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions andopenssl
# limitations under the License.
#

name "openssl3"

license "Apache-2.0"
license_file "LICENSE.txt"
skip_transitive_dependency_licensing true

dependency "cacerts"

default_version "3.5.4"

source url: "https://www.openssl.org/source/openssl-#{version}.tar.gz", extract: :lax_tar

version("3.5.4") { source sha256: "967311f84955316969bdb1d8d4b983718ef42338639c621ec4c34fddef355e99" }

relative_path "openssl-#{version}"

build do
  if !fips_mode?
    # OpenSSL on Windows now gets installed as part of the Python install, so we don't need to do anything here
    if !windows?
      command_on_repo_root "bazelisk run -- @openssl//:install --destdir=#{install_dir}/embedded"
      lib_extension = if linux_target? then ".so" else ".dylib" end
      command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix #{install_dir}/embedded" \
        " #{install_dir}/embedded/lib/libssl#{lib_extension}" \
        " #{install_dir}/embedded/lib/libcrypto#{lib_extension}" \
        " #{install_dir}/embedded/lib/pkgconfig/*.pc"
    end
  else

    env = with_standard_compiler_flags(with_embedded_path)
    if windows?
      # XXX: OpenSSL explicitly sets -march=i486 and expects that to be honored.
      # It has OPENSSL_IA32_SSE2 controlling whether it emits optimized SSE2 code
      # and the 32-bit calling convention involving XMM registers is...  vague.
      # Do not enable SSE2 generally because the hand optimized assembly will
      # overwrite registers that mingw expects to get preserved.
      env["CFLAGS"] = "-I#{install_dir}/embedded/include"
      env["CPPFLAGS"] = env["CFLAGS"]
      env["CXXFLAGS"] = env["CFLAGS"]
    end

    configure_args = []
    if mac_os_x?
      configure_cmd = "./Configure"
      configure_args << "darwin64-#{arm_target? ? "arm64" : "x86_64"}-cc"
    elsif windows?
      configure_cmd = "perl.exe ./Configure"
      configure_args << (windows_arch_i386? ? "mingw" : "mingw64")
    else
      configure_cmd = "./config"
    end

    configure_args << [
      "--libdir=lib",
      "no-idea",
      "no-mdc2",
      "no-rc5",
      "shared",
      "no-ssl3",
      "no-gost",
    ]

    if windows?
      configure_args << [
        "--prefix=#{python_3_embedded}",
        "no-zlib",
        "no-uplink",
      ]
      if ENV["AGENT_FLAVOR"] == "fips"
        configure_args << '--openssldir="C:/Program Files/Datadog/Datadog Agent/embedded3/ssl"'
        # Provide a context name for our configuration through the registry
        configure_args << "-DOSSL_WINCTX=datadog-fips-agent"
      end
    else
      configure_args << [
        "--prefix=#{install_dir}/embedded",
        "--with-zlib-lib=#{install_dir}/embedded/lib",
        "--with-zlib-include=#{install_dir}/embedded/include",
        "zlib",
      ]
    end

    # Out of abundance of caution, we put the feature flags first and then
    # the crazy platform specific compiler flags at the end.
    configure_args << env["CFLAGS"] << env["LDFLAGS"]

    # We don't use the regular configure wrapper function here since openssl's configure
    # is not the usual autoconf configure but something handmade written in perl
    command "#{configure_cmd} #{configure_args.join(' ')}", env: env

    command "make depend", env: env
    command "make -j #{workers}", env: env
    command "make install_sw install_ssldirs", env: env

    delete "#{install_dir}/embedded/bin/c_rehash"
    unless windows?
      # Remove openssl static libraries here as we can't disable those at build time
      delete "#{install_dir}/embedded/lib/libcrypto.a"
      delete "#{install_dir}/embedded/lib/libssl.a"
    else
    end
    end
  end
