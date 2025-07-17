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
default_version "0.2.2"

source url: "https://pyyaml.org/download/libyaml/yaml-#{version}.tar.gz"
source sha256: "4a9100ab61047fd9bd395bcef3ce5403365cafd55c1e0d0299cde14958e47be9"

relative_path "yaml-#{version}"

dependency "config_guess"

env = with_embedded_path
env = with_standard_compiler_flags(env)

build do
  license "MIT"
  license_file "./LICENSE"

  update_config_guess(target: "config")

  configure_options = [
    " --enable-shared",
    " --disable-static",
  ]
  configure(*configure_options, env: env)
  command "make -j #{workers}", env: env
  command "make -j #{workers} install", env: env
end
