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

# Omnibus breaks if there is no version on elements. You get an error like
# Software must specify a `version; to cache it in S3 (cacerts[/go/src/github.com/DataDog/datadog-agent/omnibus/config/software/cacerts.rb])!
# This is cryptic, and not flagged as an erro.
default_version "2025-08-12"

# relative_path "cacerts-#{version}"
# IMHO, this should be equivalant to a chdir to that directory, but it is not.
# We need to actually do the CD from within each command clause.
# relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license "MPL-2.0"
  license_file "https://www.mozilla.org/media/MPL/2.0/index.815ca599c9df.txt"

  if windows?
    command "bazelisk run -- //deps/cacerts:install --destdir='#{python_3_embedded}'", \
      cwd: "#{Omnibus::Config.project_root()}/.."
    # For debugging. Keep until we delete this file
    command "dir #{python_3_embedded}/embedded/ssl", live_stream: true
  else
    command "bazelisk run -- //deps/cacerts:install --destdir='#{install_dir}/embedded'", \
      cwd: "#{Omnibus::Config.project_root()}/.."
    # For debugging. Keep until we delete this file
    command "ls -lR #{install_dir}/embedded", live_stream: true
  end
end
