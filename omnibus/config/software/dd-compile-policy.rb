#
# Copyright 2023-2025 Datadog, Inc.
#
# Licensed under the BSD-3-Clause License (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://opensource.org/licenses/BSD-3-Clause
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
require './lib/ostools.rb'

name "dd-compile-policy"
default_version "0.0.1-release-test" # TODO: update to a stable release

# Define URLs for each platform
dd_compile_policy_urls = {
  "linux" => {
    "amd64" => "https://github.com/DataDog/dd-policy-engine/releases/download/v#{version}/dd-compile-policy-linux-amd64.tar.gz",
    "arm64" => "https://github.com/DataDog/dd-policy-engine/releases/download/v#{version}/dd-compile-policy-linux-arm64.tar.gz",
  },
  # TODO: add Windows, maybe Darwin
}

# Define SHA256 checksums for verification
# Note: These should be updated for each version
dd_compile_policy_sha256 = {
  "linux" => {
    "amd64" => "49cfee9d700e040bd99bb7a6239c667a59ac20cafcf747f261637246b0c39870",
    "arm64" => "eb87b70f55b4e3bd8a2cbd3323d1de687e03d2df3bd4ecb5456aa532e4ad8a3e"
  },
}

current_platform = if osx_target?
  "darwin"
elsif windows_target?
  "windows"
else
  "linux"
end

current_arch = if arm_target?
  "arm64"
else
  "amd64"
end

# Pick correct URL and sha256
source url: dd_compile_policy_urls[current_platform][current_arch],
       sha256: dd_compile_policy_sha256[current_platform][current_arch]

build do
  license "BSD-3-Clause"
  license_file "https://raw.githubusercontent.com/DataDog/dd-policy-engine/master/LICENSE"

  if linux?
    # Extract the tar.gz file
    target = "#{install_dir}/embedded/bin/dd-compile-policy"
    copy "#{project_dir}/dd-compile-policy", target
    block { File.chmod(0555, target) } # World executable
  end
end
