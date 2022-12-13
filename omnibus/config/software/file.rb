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

name "file"
default_version "5.39"

dependency 'zlib'
dependency 'bzip2'
dependency 'liblzma'

license "BSD"
license_file "COPYING"
skip_transitive_dependency_licensing true

version("5.39") { source sha256: "f05d286a76d9556243d0cb05814929c2ecf3a5ba07963f8f70bfaaa70517fad1" }

source url: "http://ftp.astron.com/pub/file/file-#{version}.tar.gz"

relative_path "file-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_options = []
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
