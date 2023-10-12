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

name "util-linux"
default_version "2.39.2"

license "GPLv2"
license_file "COPYING"
skip_transitive_dependency_licensing true

ship_source_offer true

version '2.39.2' do
  source url: "https://mirrors.edge.kernel.org/pub/linux/utils/util-linux/v2.39/util-linux-#{version}.tar.gz",
         sha256: 'c8e1a11dd5879a2788973c73589fbcf08606e85aeec095e516162495ead8ba68'
end

relative_path "util-linux-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "static-assert.patch", env: env # define static_assert in xxhash.h, since it's not defined in our old glibc's assert.h

  configure_options = [
    "--disable-nls",
    "--disable-asciidoc",
    "--disable-all-programs",
    "--enable-libblkid",
  ]
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "-j #{workers} install", env: env
end
