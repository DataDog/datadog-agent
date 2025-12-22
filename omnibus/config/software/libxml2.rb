#
# Copyright:: Chef Software Inc.
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

name "libxml2"
default_version "2.14.5"

license "MIT"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "zlib"

# version_list: url=https://download.gnome.org/sources/libxml2/2.14/ filter=*.tar.xz
version("2.14.5") { source sha256: "03d006f3537616833c16c53addcdc32a0eb20e55443cba4038307e3fa7d8d44b" }

source url: "https://download.gnome.org/sources/libxml2/2.14/libxml2-#{version}.tar.xz"

relative_path "libxml2-#{version}"

build do
  command_on_repo_root "bazelisk run -- @libxml2//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/libxml-2.0.pc" \
    " #{install_dir}/embedded/lib/libxml2.so"
end
