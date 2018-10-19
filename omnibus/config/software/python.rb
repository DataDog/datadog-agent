#
# Copyright 2013-2015 Chef Software, Inc.
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

name "python"
default_version "2.7.9"

license "Python-2.0"
license_file "LICENSE"
skip_transitive_dependency_licensing true

dependency "ncurses"
dependency "zlib"
dependency "openssl"
dependency "bzip2"

version("2.7.14") { source sha256: "304c9b202ea6fbd0a4a8e0ad3733715fbd4749f2204a9173a58ec53c32ea73e8" }
version("2.7.13") { source md5: "17add4bf0ad0ec2f08e0cae6d205c700" }
version("2.7.11") { source md5: "6b6076ec9e93f05dd63e47eb9c15728b" }
version("2.7.9") { source md5: "5eebcaa0030dc4061156d3429657fb83" }
version("2.7.5") { source md5: "b4f01a1d0ba0b46b05c73b2ac909b1df" }

source url: "https://python.org/ftp/python/#{version}/Python-#{version}.tgz"

relative_path "Python-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  # # github.com/DataDog/datadog-agent/vendor/github.com/sbinet/go-python
  # /usr/bin/ld: /opt/datadog-agent/embedded/lib/libpython2.7.a(abstract.o): relocation R_ARM_THM_MOVW_ABS_NC against `_Py_NotImplementedStruct' can not be used when making a shared object; recompile with -fPIC
  # /opt/datadog-agent/embedded/lib/libpython2.7.a: error adding symbols: Bad value
  # collect2: error: ld returned 1 exit status
  env['CFLAGS'] = "#{env['CFLAGS']} -fPIC"

  if mac_os_x?
    os_x_release = ohai["platform_version"].match(/([0-9]+\.[0-9]+).*/).captures[0]
    env["MACOSX_DEPLOYMENT_TARGET"] = os_x_release
  end

  command "./configure" \
          " --prefix=#{install_dir}/embedded" \
          " --enable-shared" \
          " --with-dbmliborder=", env: env

  make env: env
  make "install", env: env

  # There exists no configure flag to tell Python to not compile readline
  delete "#{install_dir}/embedded/lib/python2.7/lib-dynload/readline.*"

  # Ditto for sqlite3
  delete "#{install_dir}/embedded/lib/python2.7/lib-dynload/_sqlite3.*"
  delete "#{install_dir}/embedded/lib/python2.7/sqlite3/"

  # Remove unused extension which is known to make healthchecks fail on CentOS 6
  delete "#{install_dir}/embedded/lib/python2.7/lib-dynload/_bsddb.*"

  # Remove sqlite3 libraries, if you want to include sqlite, create a new def
  # in your software project and build it explicitly. This removes the adapter
  # library from python, which links incorrectly to a system library. Adding
  # your own sqlite definition will fix this.
  delete "#{install_dir}/embedded/lib/python2.7/lib-dynload/_sqlite3.*"
end
