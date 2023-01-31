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
default_version "3.40.1"

dependency 'libedit'
dependency 'zlib'

license "Public Domain"
skip_transitive_dependency_licensing true

version("3.40.1") do
  source url: "https://www.sqlite.org/2022/sqlite-autoconf-3400100.tar.gz",
         sha256: "2c5dea207fa508d765af1ef620b637dcb06572afa6f01f0815bd5bbf864b33d9"
end

relative_path "sqlite-autoconf-3400100"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CFLAGS"] << " -fPIC"

  configure "--enable-pic", env: env

  make "-j #{workers}", env: env
  make "install", env: env
end
