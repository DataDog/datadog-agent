#
# Copyright 2012-2014 Chef Software, Inc.
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

name "rpm"
default_version "4.18.1"

license "LGPLv2"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "config_guess"
dependency "popt"
dependency "libsqlite3"

ship_source_offer true

version "4.18.1" do
  source url: "http://ftp.rpm.org/releases/rpm-4.18.x/rpm-#{version}.tar.bz2",
         sha256: "37f3b42c0966941e2ad3f10fde3639824a6591d07197ba8fd0869ca0779e1f56"
end

relative_path "rpm-#{version}"

build do
  command_on_repo_root "bazelisk run -- @rpm//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/rpm.pc" \
    " #{install_dir}/embedded/lib/librpm.so" \
    " #{install_dir}/embedded/lib/librpmio.so"
end
