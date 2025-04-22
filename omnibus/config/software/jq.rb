# Copyright:: Copyright (c) 2012-2014 Chef Software, Inc.
# License:: Apache License, Version 2.0
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

name "jq"
default_version "1.7.1"

# Build dependencies
dependency "preparation"

source url: "https://github.com/jqlang/jq/archive/refs/tags/jq-#{version}.tar.gz",
       sha256: "fc75b1824aba7a954ef0886371d951c3bf4b6e0a921d1aefc553f309702d6ed1"

relative_path "jq-jq-#{version}"

build do
  license "JQ"
  license_file "https://raw.githubusercontent.com/jqlang/jq/refs/heads/master/COPYING"
  env = with_standard_compiler_flags(with_embedded_path)

  # Download and extract oniguruma
    #   command "mkdir -p modules/oniguruma", env: env
    #   command "cd modules/oniguruma && curl -L https://github.com/kkos/oniguruma/archive/refs/tags/v6.9.8.tar.gz | tar xz --strip-components=1", env: env

  # Generate configure script
  command "autoreconf -i", env: env

  # Configure with standard options
  configure_options = [
    "--disable-maintainer-mode",
    "--disable-dependency-tracking",
    "--disable-silent-rules",
    "--disable-docs",
    "--disable-valgrind",
    "--with-oniguruma=no",
    "--disable-shared",
    "--enable-static"
  ]

  configure(*configure_options, env: env)

  # Build and install
  command "make -j #{workers}", env: env
  command "make install-binPROGRAMS", env: env
end
