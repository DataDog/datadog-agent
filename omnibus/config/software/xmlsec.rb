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
default_version "1.3.1"

license "MIT"
license_file "Copyright"
skip_transitive_dependency_licensing true

dependency "libxml2"
dependency "libxslt"
dependency "libtool"
dependency "libgcrypt"
dependency "gnutls"
dependency ENV["OMNIBUS_OPENSSL_SOFTWARE"] || "openssl"

version("1.3.1") { source sha256: "10f48384d4fd1afc05fea545b74fbf7c152582f0a895c189f164d55270400c63" }

source url: "https://github.com/lsh123/xmlsec/releases/download/xmlsec_1_3_1/xmlsec1-#{version}.tar.gz"

relative_path "xmlsec1-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"
  env["CFLAGS"] << " -fPIC"
  env["CFLAGS"] << " -std=c99"

  update_config_guess

  command "./configure" \
          " --prefix=#{install_dir}/embedded" \
          " --enable-docs" \
          " --disable-static", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
