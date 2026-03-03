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

# Note: we need to bundle recent cacerts to make the Agent trust our backend.
# Not shipping the cacerts causes the following error in the Docker Agent:
#   Error while processing transaction: error while sending transaction, rescheduling it:
#   Post "https://7-31-0-app.agent.datadoghq.com/intake/?api_key=********************************":
#   x509: certificate signed by unknown authority
# because the Docker Agent image doesn't have other system SSL certificates.
# Even though these cacerts might become outdated in the future, some python
# dependencies we ship also bundle cacerts so we aren't making things worse by
# doing this.
name "cacerts"

# We have a synthetic monitor on the latest cacerts file to warn us when the latest
# cacerts bundle changes.
# This allows us to always use up-to-date cacerts, without breaking all builds
# when they change.
default_version "2025-12-02"
source url: "https://curl.se/ca/cacert-#{version}.pem",
       sha256: "f1407d974c5ed87d544bd931a278232e13925177e239fca370619aba63c757b4",
       target_filename: "cacert.pem"

relative_path "cacerts-#{version}"

build do
  license "MPL-2.0"
  license_file "https://www.mozilla.org/media/MPL/2.0/index.815ca599c9df.txt"

  if windows?
    mkdir "#{python_3_embedded}/ssl/certs"
    copy "#{project_dir}/cacert.pem", "#{python_3_embedded}/ssl/certs/cacert.pem"
    copy "#{project_dir}/cacert.pem", "#{python_3_embedded}/ssl/cert.pem"
  else
    mkdir "#{install_dir}/embedded/ssl/certs"
    copy "#{project_dir}/cacert.pem", "#{install_dir}/embedded/ssl/certs/cacert.pem"
    block 'set certificate permissions and relative symlink within embedded SSL configuration' do
      Dir.chdir "#{install_dir}/embedded/ssl" do
        File.chmod 0644, 'certs/cacert.pem'
        File.symlink 'certs/cacert.pem', 'cert.pem'
      end
    end
  end
end
