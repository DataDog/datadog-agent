#
# Copyright 2012-2015 Chef Software, Inc.
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

name "libffi"

default_version "3.4.8"

license "MIT"
license_file "LICENSE"
skip_transitive_dependency_licensing true

# Is libtool actually necessary? Doesn't configure generate one?
dependency "libtool" unless windows?

version("3.4.8") { source sha256: "bc9842a18898bfacb0ed1252c4febcc7e78fa139fd27fdc7a3e30d9d9356119b" }

source url: "https://github.com/libffi/libffi/releases/download/v#{version}/libffi-#{version}.tar.gz"

relative_path "libffi-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["INSTALL"] = "/opt/freeware/bin/install" if aix?

  configure_command = ["--disable-static", "--disable-docs", "--disable-multi-os-directory"]

  configure(*configure_command, env: env)

  if solaris_10?
    # run old make :(
    make env: env, bin: "/usr/ccs/bin/make"
    make "install", env: env, bin: "/usr/ccs/bin/make"
  else
    make "-j #{workers}", env: env
    make "-j #{workers} install", env: env
  end
end
