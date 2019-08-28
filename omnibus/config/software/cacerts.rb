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

name "cacerts"

# We always pull the latest version,
# so the hashsum check will break every time the file is updated on the remote
default_version "latest"

source url: "https://curl.haxx.se/ca/cacert.pem",
       sha256: "38b6230aa4bee062cd34ee0ff6da173250899642b1937fc130896290b6bd91e3",
       target_filename: "cacert.pem"

relative_path "cacerts-#{version}"

build do
  ship_license "https://www.mozilla.org/media/MPL/2.0/index.815ca599c9df.txt"

  mkdir "#{install_dir}/embedded/ssl/certs"

  copy "#{project_dir}/cacert.pem", "#{install_dir}/embedded/ssl/certs/cacert.pem"
  copy "#{project_dir}/cacert.pem", "#{install_dir}/embedded/ssl/cert.pem" if windows?

  # Windows does not support symlinks
  unless windows?
    link "#{install_dir}/embedded/ssl/certs/cacert.pem", "#{install_dir}/embedded/ssl/cert.pem"

    block { File.chmod(0644, "#{install_dir}/embedded/ssl/certs/cacert.pem") }
  end
end
