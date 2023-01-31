#
# Copyright:: Chef Software, Inc.
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

name "elfutils"
default_version "0.188"

dependency 'm4'
dependency 'zlib'
dependency 'liblzma'
dependency 'bzip2'

license "LGPLv3+"
license_file "COPYING-LGPLV3"
skip_transitive_dependency_licensing true

version("0.188") { source sha256: "fb8b0e8d0802005b9a309c60c1d8de32dd2951b56f0c3a3cb56d21ce01595dff" }

source url: "https://sourceware.org/elfutils/ftp/#{version}/elfutils-#{version}.tar.bz2"

relative_path "elfutils-#{version}"

build do
    env = with_standard_compiler_flags(with_embedded_path)

    configure_options = [
      "--prefix=#{install_dir}/embedded",
      "--disable-debuginfod",
      "--disable-libdebuginfod",
      "--disable-nls",
      "--enable-pic"
    ]
  
    configure(*configure_options, env: env)
  
    make "-j #{workers}", env: env
    make "install", env: env
  end
