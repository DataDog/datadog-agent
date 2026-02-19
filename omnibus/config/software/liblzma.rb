#
# Copyright 2014-2018 Chef Software, Inc.
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

name "liblzma"
default_version "5.4.2"

license "Public-Domain"
license_file "COPYING"
skip_transitive_dependency_licensing true

# version_list: url=http://tukaani.org/xz/ filer=*.tar.gz

version("5.4.2") { source sha256: "87947679abcf77cc509d8d1b474218fd16b72281e2797360e909deaee1ac9d05" }

source url: "https://tukaani.org/xz/xz-#{version}.tar.gz"

relative_path "xz-#{version}"

build do
  command_on_repo_root "bazelisk run -- @xz//:install --destdir='#{install_dir}/embedded'"

  sh_lib = if linux_target? then "liblzma.so" else "liblzma.dylib" end
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' " \
    "#{install_dir}/embedded/lib/#{sh_lib}"
end
