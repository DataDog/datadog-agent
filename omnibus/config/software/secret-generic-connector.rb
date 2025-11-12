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
default_version "1.3.3"

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
    "amd64" => "845efbf48b108981a9495a219af944194641fbcc13f1648cd953443d73a08754",
    "arm64" => "2a972ec71d398bdaf574a30a5adcf26bcfce7e355bb8c1eb7fe9d0db4eb27e01"
  },
  "windows" => {
    "amd64" => "2c7a9960abab52c2a3a1678bd91806842c5477805defea9099893b4d4cb09bad",
    "arm64" => "0b26608f8a252a8133f499dafea9cb44f14a71fa8bb924b3a49c80336c0b660d"
  },
  "darwin" => {
    "amd64" => "ac92a4ec5432f841b27019e670e5fe3bdd8683ee0e6dfd378e907454f33f9779",
    "arm64" => "f7cd78ff0baf47022749a8846a3ce9c722d058d287a94b975059233f8a1a27aa"
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
