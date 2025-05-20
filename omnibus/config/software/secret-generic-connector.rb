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
default_version "0.2.2"

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
    "amd64" => "f0ad4c07b48c189f176ca7c8cb436db9372887603c067b1c9289bf3831a46081",
    "arm64" => "cec90581e889b2e602ec09d5e08c735d3997d0e02c2c46b61f8ea06ab686343e"
  },
  "windows" => {
    "amd64" => "c6c179dbb031b24d81c66579174321400a9d68f596cda2a46f86191dae06719a",
    "arm64" => "15c083b6a2679c6bb3030a29be13da2321934ed7597451acfd8ab8bb91a3ca73"
  },
  "darwin" => {
    "amd64" => "cdc2a70af5113d944411f53698af2024c23229ad351098b3cdc4628c90fbf7ea",
    "arm64" => "5eeacd816b243ea69c0514d9ba6d015ce9d1bfa57b408f49b34d6f6c7f4789e8"
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
    copy "#{project_dir}/datadog-secret-backend.exe", "#{install_dir}/bin/secret-generic-connector.exe"
  else
    # Extract the tar.gz file
    target = "#{install_dir}/embedded/bin/secret-generic-connector"
    copy "#{project_dir}/datadog-secret-backend", target
    block { File.chmod(0500, target) }
  end
end
