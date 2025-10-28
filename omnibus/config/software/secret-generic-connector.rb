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
default_version "1.3.1"

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
    "amd64" => "00b86b9426912caf135256a69e9da1aa33440adf5414b79e7ebf56c0f2678fcf",
    "arm64" => "bdec03a782023d7685488be3d71a556a8c2d5b56a2ab30ef5b10fd33c97d2ea7"
  },
  "windows" => {
    "amd64" => "0028ea1c6f37996fe9b752c1ec913aca5af3406571f54a6ecc0129259b504bd8",
    "arm64" => "5fec5469cdd386086b4092c09a46bd5cf554e34d5f852a5d85001f1f208571fd"
  },
  "darwin" => {
    "amd64" => "43b927e1781ddb3b629781ba3a5ef680c2ef8cfcb34d73a29c3403d00acd5bc9",
    "arm64" => "ba9160fb627f8e61d389078eed655546376bbe64c9f73ff06be7a2b1fd275a44"
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
    mkdir "#{install_dir}/bin/agent"
    # Extract the zip file
    copy "#{project_dir}/datadog-secret-backend.exe", "#{install_dir}/bin/agent/secret-generic-connector.exe"
  else
    # Extract the tar.gz file
    target = "#{install_dir}/embedded/bin/secret-generic-connector"
    copy "#{project_dir}/datadog-secret-backend", target
    block { File.chmod(0500, target) }
  end
end
