#
# Copyright 2012-2019, Chef Software Inc.
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

name "libselinux"
default_version "3.5"

dependency 'pcre2'
dependency 'libsepol'

license "Public-Domain"
license_file "LICENSE"
skip_transitive_dependency_licensing true

version("3.5") { source sha256: "9a3a3705ac13a2ccca2de6d652b6356fead10f36fb33115c185c5ccdf29eec19" }

source url: "https://github.com/SELinuxProject/selinux/releases/download/#{version}/libselinux-#{version}.tar.gz"

relative_path "#{name}-#{version}"

build do
  command_on_repo_root "bazelisk run -- @libselinux//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/libselinux.pc" \
    " #{install_dir}/embedded/lib/libselinux.so"
end
