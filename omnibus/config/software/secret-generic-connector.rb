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

name "secret-generic-connector"
default_version "1.1.0"

# Define URLs for each platform
secret_generic_urls = {
  "linux" => {
    "amd64" => "https://github.com/DataDog/datadog-secret-backend/releases/download/v#{version}/datadog-secret-backend-linux-amd64.tar.gz",
    "arm64" => "https://github.com/DataDog/datadog-secret-backend/releases/download/v#{version}/datadog-secret-backend-linux-arm64.tar.gz",
  },
  "windows" => {
    "amd64" => "https://github.com/DataDog/datadog-secret-backend/releases/download/v#{version}/datadog-secret-backend-windows-amd64.zip",
    "arm64" => "https://github.com/DataDog/datadog-secret-backend/releases/download/v#{version}/datadog-secret-backend-windows-arm64.zip",
  },
  "darwin" => {
    "amd64" => "https://github.com/DataDog/datadog-secret-backend/releases/download/v#{version}/datadog-secret-backend-darwin-amd64.tar.gz",
    "arm64" => "https://github.com/DataDog/datadog-secret-backend/releases/download/v#{version}/datadog-secret-backend-darwin-arm64.tar.gz",
  },
}

# Define SHA256 checksums for verification
# Note: These should be updated for each version
secret_generic_sha256 = {
  "linux" => {
    "amd64" => "85a2ec8709981e450c95158fa89ecc26a80e19f8fd8f0b338c5d71e762e1cc09",
    "arm64" => "d1ad920f90714a9a8ca8ce3161fd77afbf0cadb1e9aeea24d7cc771d59bef2e0"
  },
  "windows" => {
    "amd64" => "ef35a5a4b4f8d03d43af16e7c1f1b33d1864c9a4fdb11ae839036db037df1bae",
    "arm64" => "6e01289f696cbcda2e51bb2f51b28e804b95bffad9e5580a159c9e3c9394653d"
  },
  "darwin" => {
    "amd64" => "775d0d00c66991b523a29047f67ccdcb1ff7f16891e5aaaae24693ad18f5e3d4",
    "arm64" => "7712d5d748caf7931ed1c48dfb30311395c17163b7aca3d3d1a59450c1d006e8"
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
source url: secret_generic_urls[current_platform][current_arch],
       sha256: secret_generic_sha256[current_platform][current_arch]

build do
  license "BSD-3-Clause"
  license_file "https://raw.githubusercontent.com/DataDog/datadog-secret-backend/master/LICENSE"

  if windows?
    # Extract the zip file
    copy "#{project_dir}/datadog-secret-backend.exe", "#{install_dir}/bin/agent/secret-generic-connector.exe"
  else
    # Extract the tar.gz file
    target = "#{install_dir}/embedded/bin/secret-generic-connector"
    copy "#{project_dir}/datadog-secret-backend", target
    block { File.chmod(0500, target) }
  end
end
