#
# Copyright:: Chef Software, Inc.
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
# See the License for the specific language governing permissions and
# limitations under the License.
#

name "xmlsec"
default_version "1.2.37"

license "BSD-2-Clause"
license_file "LICENCE"
skip_transitive_dependency_licensing true

dependency "libxml2"
dependency "libxslt"
dependency "openssl"
dependency "libtool"

version("1.2.37") { source sha256: "5f8dfbcb6d1e56bddd0b5ec2e00a3d0ca5342a9f57c24dffde5c796b2be2871c" }

source url: "https://github.com/lsh123/xmlsec/releases/download/xmlsec-1_2_37/xmlsec1-#{version}.tar.gz"

relative_path "xmlsec1-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CFLAGS"] << " -fPIC"
  env["CFLAGS"] << " -std=c99"

  update_config_guess

  command "./configure" \
          " --prefix=#{install_dir}/embedded", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
