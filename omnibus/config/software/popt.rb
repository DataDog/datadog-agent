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
default_version "1.18"

license "MIT"
license_file "COPYING"
skip_transitive_dependency_licensing true

dependency "config_guess"

version "1.18" do
  source url: "http://ftp.rpm.org/popt/releases/popt-1.x/popt-#{version}.tar.gz",
         sha256: "5159bc03a20b28ce363aa96765f37df99ea4d8850b1ece17d1e6ad5c24fdc5d1"
end

version("1.16") do
  source url: "http://ftp.rpm.org/popt/releases/historical/popt-#{version}.tar.gz",
         sha256: "e728ed296fe9f069a0e005003c3d6b2dde3d9cad453422a10d6558616d304cc8"
end

relative_path "popt-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CFLAGS"] << " -fPIC"

  update_config_guess

  if version == "1.16" && (ppc64? || ppc64le?)
    patch source: "v1.16.ppc64le-configure.patch", plevel: 1
  end

  configure_options = [
    "--disable-nls",
  ]
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
