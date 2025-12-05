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

default_version "3.5.4"

source url: "https://www.openssl.org/source/openssl-#{version}.tar.gz", extract: :lax_tar

version("3.5.4") { source sha256: "967311f84955316969bdb1d8d4b983718ef42338639c621ec4c34fddef355e99" }

relative_path "openssl-#{version}"

build do
  # OpenSSL on Windows now gets installed as part of the Python install, so we don't need to do anything here
  if !windows?
    if ENV["AGENT_FLAVOR"] == "fips"
      fips_flag = "--//:fips_mode=true"
    else
      # We could set here //:fips_mode=false, but it's not necessary because the default is false.
      fips_flag = ""
    end
    command_on_repo_root "bazelisk run -- @openssl//:install --destdir=#{install_dir}/embedded #{fips_flag}"
    lib_extension = if linux_target? then ".so" else ".dylib" end
    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix #{install_dir}/embedded" \
      " #{install_dir}/embedded/lib/libssl#{lib_extension}" \
      " #{install_dir}/embedded/lib/libcrypto#{lib_extension}" \
      " #{install_dir}/embedded/lib/pkgconfig/*.pc"
  end
end