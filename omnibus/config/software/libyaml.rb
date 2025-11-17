#
# Copyright:: Copyright (c) 2012-2014 Chef Software, Inc.
# License:: Apache License, Version 2.0
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

name "libyaml"
default_version "0.2.5"

license "MIT"
license_file "./LICENSE"

source url: "https://pyyaml.org/download/libyaml/yaml-#{version}.tar.gz"
source sha256: "c642ae9b75fee120b2d96c712538bd2cf283228d2337df2cf2988e3c02678ef4"

relative_path "yaml-#{version}"

build do
  command_on_repo_root "bazelisk run -- @libyaml//:install --destdir='#{install_dir}/embedded'"
  sh_lib = if linux_target? then "libyaml.so" else "libyaml.dylib" end
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' " \
    "#{install_dir}/embedded/lib/pkgconfig/yaml-0.1.pc " \
    "#{install_dir}/embedded/lib/#{sh_lib}"
end
