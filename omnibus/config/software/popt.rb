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

name "popt"
default_version "1.19"

license "MIT"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "config_guess"

version "1.19" do
  source url: "http://ftp.rpm.org/popt/releases/popt-1.x/popt-#{version}.tar.gz",
         sha256: "c25a4838fc8e4c1c8aacb8bd620edb3084a3d63bf8987fdad3ca2758c63240f9"
end

relative_path "popt-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CFLAGS"] << " -fPIC"

  update_config_guess

  configure_options = [
    "--prefix=#{install_dir}/embedded",
    "--disable-static",
    "--disable-nls",
  ]
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
