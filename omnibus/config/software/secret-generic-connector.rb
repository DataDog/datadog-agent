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
default_version "0.2.0"

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
    "amd64" => "d4213e92b7810c44bd7e0a39338c0fb4f9302dda65d0dba673558ba7d6a007bc",
    "arm64" => "eb91cc9db52bd22be3b4ea75c6d94226c648158c2c0b77b60fd0d606e716a328"
  },
  "windows" => {
    "amd64" => "32c51115d1c46a225bca27c355a968eacf239870d31cc96888e61a9f939f738f",
    "arm64" => "60aeb056907ddc95c0e21df04b8732b3b240dd532d6ac31d2b617899810db55d"
  },
  "darwin" => {
    "amd64" => "c03fed7465989aae1c210bf6464eb2b06fd523f7af8a4e840735e3e8a116c4fd",
    "arm64" => "8ec77da90910db5f355078a5f1df9bcbebbfb7618c1d23af2460abb4b3e47b4d"
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