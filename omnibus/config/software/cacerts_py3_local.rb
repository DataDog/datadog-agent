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

name "cacerts_py3_local"

# We always pull the latest version,
# so the hashsum check will break every time the file is updated on the remote
default_version "latest"

source url: "https://curl.haxx.se/ca/cacert.pem",
       sha256: "bb28d145ed1a4ee67253d8ddb11268069c9dafe3db25a9eee654974c4e43eee5",
       target_filename: "cacert.pem"

relative_path "cacerts-#{version}"

build do
  ship_license "https://www.mozilla.org/media/MPL/2.0/index.815ca599c9df.txt"

  mkdir "#{python_3_embedded}/ssl/certs"

  copy "#{project_dir}/cacert.pem", "#{python_3_embedded}/ssl/certs/cacert.pem"
  copy "#{project_dir}/cacert.pem", "#{python_3_embedded}/ssl/cert.pem" if windows?

  # Windows does not support symlinks
  unless windows?
    link "#{python_3_embedded}/ssl/certs/cacert.pem", "#{python_3_embedded}/ssl/cert.pem"

    block { File.chmod(0644, "#{python_3_embedded}/ssl/certs/cacert.pem") }
  end
end
