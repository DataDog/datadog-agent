#
# Copyright 2016 Chef Software, Inc.
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

name "lua"
default_version "5.4.6"

version("5.4.6") { source sha256: "7d5ea1b9cb6aa0b59ca3dde1c6adcb57ef83a1ba8e5432c0ecd06bf439b3ad88" }

license "MIT"
license_file "https://www.lua.org/license.html"
skip_transitive_dependency_licensing true

source url: "https://www.lua.org/ftp/lua-#{version}.tar.gz"

relative_path "lua-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "nodoc.patch", env: env # don't install documentation

  # lua compiles in a slightly interesting way, it has minimal configuration options
  # and only provides a makefile. We can't use use `-DLUA_USE_LINUX` or the `make linux`
  # methods because they make the assumption that the readline package has been installed.
  mycflags = "-I#{install_dir}/embedded/include -O2 -DLUA_USE_POSIX -DLUA_USE_DLOPEN -fpic"
  myldflags = "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib"
  mylibs = "-ldl"
  make "all MYCFLAGS='#{mycflags}' MYLDFLAGS='#{myldflags}' MYLIBS='#{mylibs}'", env: env, cwd: "#{project_dir}/src"
  make "-j #{workers} install INSTALL_TOP=#{install_dir}/embedded", env: env
end
