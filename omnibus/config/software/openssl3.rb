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

dependency "zlib" unless windows?
dependency "cacerts"

default_version "3.4.1"

source url: "https://www.openssl.org/source/openssl-#{version}.tar.gz", extract: :lax_tar

version("3.1.0") { source sha256: "aaa925ad9828745c4cad9d9efeb273deca820f2cdcf2c3ac7d7c1212b7c497b4" }
version("3.0.9") { source sha256: "eb1ab04781474360f77c318ab89d8c5a03abc38e63d65a603cabbf1b00a1dc90" }
version("3.0.8") { source sha256: "6c13d2bf38fdf31eac3ce2a347073673f5d63263398f1f69d0df4a41253e4b3e" }
version("3.0.11") { source sha256: "b3425d3bb4a2218d0697eb41f7fc0cdede016ed19ca49d168b78e8d947887f55" }
version("3.0.12") { source sha256: "f93c9e8edde5e9166119de31755fc87b4aa34863662f67ddfcba14d0b6b69b61" }
version("3.0.13") { source sha256: "88525753f79d3bec27d2fa7c66aa0b92b3aa9498dafd93d7cfa4b3780cdae313" }
version("3.0.14") { source sha256: "eeca035d4dd4e84fc25846d952da6297484afa0650a6f84c682e39df3a4123ca" }
version("3.3.0") { source sha256: "53e66b043322a606abf0087e7699a0e033a37fa13feb9742df35c3a33b18fb02" }
version("3.3.1") { source sha256: "777cd596284c883375a2a7a11bf5d2786fc5413255efab20c50d6ffe6d020b7e" }
version("3.3.2") { source sha256: "2e8a40b01979afe8be0bbfb3de5dc1c6709fedb46d6c89c10da114ab5fc3d281" }
version("3.4.0") { source sha256: "e15dda82fe2fe8139dc2ac21a36d4ca01d5313c75f99f46c4e8a27709b7294bf" }
version("3.4.1") { source sha256: "002a2d6b30b58bf4bea46c43bdd96365aaf8daa6c428782aa4feee06da197df3" }

relative_path "openssl-#{version}"

build do
  patch source: "0001-fix-preprocessor-concatenation.patch"

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
