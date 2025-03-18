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

name "zlib"
default_version "1.3.1"

version "1.3.1" do
  source sha256: "9a93b2b7dfdac77ceba5a558a580e74667dd6fede4585b91eefb60f03b72df23"
end

source url: "https://zlib.net/fossils/zlib-#{version}.tar.gz",
       extract: :seven_zip

relative_path "zlib-#{version}"

build do
  license "Zlib"
  license_file "https://gist.githubusercontent.com/remh/77877aa00b45c1ebc152/raw/372a65de9f4c4ed376771b8d2d0943da83064726/zlib.license"

  if windows?
    env = with_standard_compiler_flags(with_embedded_path, bfd_flags: true)

    patch source: "zlib-windows-relocate.patch", env: env

    # We can't use the top-level Makefile. Instead, the developers have made
    # an organic, artisanal, hand-crafted Makefile.gcc for us which takes a few
    # variables.
    env["BINARY_PATH"] = "/bin"
    env["LIBRARY_PATH"] = "/lib"
    env["INCLUDE_PATH"] = "/include"
    env["DESTDIR"] = "#{install_dir}/embedded"

    make_args = [
      "-fwin32/Makefile.gcc",
      "SHARED_MODE=1",
      "CFLAGS=\"#{env["CFLAGS"]} -Wall\"",
      "ASFLAGS=\"#{env["CFLAGS"]} -Wall\"",
      "LDFLAGS=\"#{env["LDFLAGS"]}\"",
      "ARFLAGS=\"rcs #{env["ARFLAGS"]}\"",
      "RCFLAGS=\"--define GCC_WINDRES #{env["RCFLAGS"]}\"",
    ]

    # On windows, msys make 3.81 doesn't support -j.
    make(*make_args, env: env)
    make("install", *make_args, env: env)
  else
    # We omit the omnibus path here because it breaks mac_os_x builds by picking
    # up the embedded libtool instead of the system libtool which the zlib
    # configure script cannot handle.
    # TODO: Do other OSes need this?  Is this strictly a mac thing?
    env = with_standard_compiler_flags
    if solaris_10?
      # For some reason zlib needs this flag on solaris (cargocult warning?)
      env["CFLAGS"] << " -DNO_VIZ"
    end

    env["CFLAGS"] << " -fPIC"
    cmake env: env

    delete "#{install_dir}/embedded/lib/libz.a"
    delete "#{install_dir}/embedded/share/man/man3/zlib.3"
  end
end
