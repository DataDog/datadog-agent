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
#   Software must specify a `version; to cache it in S3 (cacerts[/go/src/github.com/DataDog/datadog-agent/omnibus/config/software/cacerts.rb])!
default_version "2025-08-12"

build do
  license "MPL-2.0"
  license_file "https://www.mozilla.org/media/MPL/2.0/index.815ca599c9df.txt"

  if windows?
    # This is a hack around pkg_install failing on windows. The exact error we get is
    #  FileNotFoundError: [WinError 206] The filename or extension is too long: 'C:\\Users\\ContainerAdministrator\\AppData\\Local\\Temp\\Bazel.runfiles_7ghfo0_s\\runfiles\\rules_python++python+python_3_11_x86_64-pc-windows-msvc\\Lib\\site-packages\\pkg_resources\\tests\\data\\my-test-package_unpacked-egg\\my_test_package-1.0-py3.7.egg\\EGG-INFO'
    # which has too many things to fix for what is basically a file copy.
    copy "deps/cacerts/cacert.pem", "#{python_3_embedded}/embedded/ssl", \
      cwd: "#{Omnibus::Config.project_root()}/.."
    copy "deps/cacerts/cacert.pem", "#{python_3_embedded}/embedded/ssl/cacerts", \
      cwd: "#{Omnibus::Config.project_root()}/.."
  else
    command "bazelisk run -- //deps/cacerts:install --destdir='#{install_dir}/embedded'", \
      cwd: "#{Omnibus::Config.project_root()}/.."
  end
end
