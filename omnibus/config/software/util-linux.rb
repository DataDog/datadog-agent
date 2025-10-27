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
  command_on_repo_root "bazelisk run -- @util-linux//:blkid_install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/blkid.pc" \
    " #{install_dir}/embedded/lib/libblkid.so"
end
