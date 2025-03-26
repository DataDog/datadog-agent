#
# Copyright 2014 Chef Software, Inc.
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

name "expat"
default_version "2.5.0"

relative_path "expat-#{version}"
dependency "config_guess"

license "MIT"
license_file "COPYING"
skip_transitive_dependency_licensing true

# version_list: url=https://github.com/libexpat/libexpat/releases filter=*.tar.gz
source url: "https://github.com/libexpat/libexpat/releases/download/R_#{version.gsub(".", "_")}/expat-#{version}.tar.gz"

version("2.5.0") { source sha256: "6b902ab103843592be5e99504f846ec109c1abb692e85347587f237a4ffa1033" }

build do
  env = with_standard_compiler_flags(with_embedded_path)

  update_config_guess(target: "conftools")

  # AIX needs two fixes to compile the latest version.
  #  1. We need to add -lm to link in the proper math declarations
  #  2. Since we are using xlc to compile, we need to use qvisibility instead of fvisibility
  #     Refer to https://www.ibm.com/docs/en/xl-c-and-cpp-aix/16.1?topic=descriptions-qvisibility-fvisibility
  if aix?
    env["LDFLAGS"] << " -lm"
    if version <= "2.4.1"
      patch source: "configure_xlc_visibility.patch", plevel: 1, env: env
    else
      patch source: "configure_xlc_visibility_2.4.7.patch", plevel: 1, env: env
    end
  end

  configure_options = [
    "--disable-static",
    " --without-examples",
    " --without-tests",
  ]
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
