#
# Copyright 2012-2015 Chef Software, Inc.
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

name "zlib"

build do
  license "Zlib"
  license_file "https://gist.githubusercontent.com/remh/77877aa00b45c1ebc152/raw/372a65de9f4c4ed376771b8d2d0943da83064726/zlib.license"

  command "bazelisk run -- @zlib//:install --destdir='#{install_dir}/embedded'", \
	cwd: "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent"
end
