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

default_version "3.5.5"

source url: "https://www.openssl.org/source/openssl-#{version}.tar.gz", extract: :lax_tar

version("3.5.5") { source sha256: "b28c91532a8b65a1f983b4c28b7488174e4a01008e29ce8e69bd789f28bc2a89" }

relative_path "openssl-#{version}"

build do
  flavor_flag = fips_mode? ? "--//packages/agent:flavor=fips" : ""

  if windows?
    command_on_repo_root "bazelisk run #{flavor_flag} -- @openssl//:install --destdir=#{install_dir}/embedded3"
  else
    command_on_repo_root "bazelisk run #{flavor_flag} -- @openssl//:install --destdir=#{install_dir}/embedded"
    # build_agent_dmg.sh sets INSTALL_DIR to some temporary folder.
    # This messes up openssl's internal paths. So we have to use another variable
    # so that replace_prefix and fix_openssl_paths set path correctly inside of the
    # openssl binaries on macos
    real_install_dir = if mac_os_x? then "/opt/datadog-agent" else install_dir end
    lib_extension = if linux_target? then ".so" else ".dylib" end

    files_to_patch = [
      "lib/libssl#{lib_extension}",
      "lib/libcrypto#{lib_extension}",
      "bin/openssl",
    ]
    if fips_mode?
      files_to_patch.append("lib/ossl-modules/*#{lib_extension}", "lib/engines-3/*#{lib_extension}")
    end

    files_to_patch = files_to_patch.map { |path| "#{install_dir}/embedded/#{path}" }

    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix #{real_install_dir}/embedded #{files_to_patch.join(' ')}"

    command_on_repo_root "bazelisk run -- //deps/openssl:fix_openssl_paths --destdir #{real_install_dir}/embedded" \
      " #{install_dir}/embedded/lib/libssl#{lib_extension}" \
      " #{install_dir}/embedded/lib/libcrypto#{lib_extension}" \
  end
  if fips_mode?
    if windows?
      command_on_repo_root "bazelisk run -- @openssl_fips//:install --destdir=#{install_dir}/embedded3"
      command_on_repo_root "bazelisk run -- @openssl_fips//:configure_fips --destdir=\"#{install_dir}/embedded3\" --embedded_ssl_dir=\"C:/Program Files/Datadog/Datadog Agent/embedded3/ssl\""
    else
      command_on_repo_root "bazelisk run -- @openssl_fips//:install --destdir=#{install_dir}/embedded"
      command_on_repo_root "bazelisk run -- @openssl_fips//:configure_fips --destdir=#{install_dir}/embedded"
      command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix #{install_dir}/embedded" \
        " #{install_dir}/embedded/lib/ossl-modules/fips.so"
    end
  end
end
