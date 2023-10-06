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

require './lib/cmake.rb'

name "zstd"
default_version "1.5.5"

license "BSD"
license_file "LICENSE"
skip_transitive_dependency_licensing true

dependency "libarchive"

version("1.5.5") { source sha256: "9c4396cc829cfae319a6e2615202e82aad41372073482fce286fac78646d3ee4" }

source url: "https://github.com/facebook/zstd/releases/download/v#{version}/zstd-#{version}.tar.gz"

relative_path "zstd-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  cmake_build_dir = "#{project_dir}/build/cmake/builddir"

  command "mkdir #{cmake_build_dir}", env: env

  cmake_options = [
    "-DZSTD_BUILD_PROGRAMS=OFF",
    "-DENABLE_STATIC=ON",
    "-DENABLE_SHARED=OFF",
  ]

  cmake(*cmake_options, env: env, cwd: cmake_build_dir, prefix: "#{install_dir}/embedded")
end
