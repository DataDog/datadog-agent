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

name "sqlite"
default_version "3.43.1"

dependency 'libedit'
dependency 'zlib'

license "Public-Domain"
skip_transitive_dependency_licensing true

version("3.43.1") do
  source url: "https://www.sqlite.org/2023/sqlite-autoconf-3430101.tar.gz",
         sha256: "098984eb36a684c90bc01c0eb7bda3273c327cbc3673d7d0bc195028c19fb7b0"
end

relative_path "sqlite-autoconf-3430100"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CFLAGS"] << " -fPIC"

  configure_opts = [
    "--enable-pic",
    "--disable-static",
    "--enable-shared",
  ]
  configure configure_opts, env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
