#
# Copyright:: Copyright (c) 2013-2014 Chef Software, Inc.
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

name "python2"

if ohai["platform"] != "windows"
  default_version "2.7.16"

  dependency "ncurses"
  dependency "zlib"
  dependency "openssl"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "libyaml"

  source :url => "http://python.org/ftp/python/#{version}/Python-#{version}.tgz",
         :sha256 => "01da813a3600876f03f46db11cc5c408175e99f03af2ba942ef324389a83bad5"

  relative_path "Python-#{version}"

  env = {
    "CFLAGS" => "-I#{install_dir}/embedded/include -O2 -g -pipe -fPIC",
    "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
  }

  python_configure = ["./configure",
                      "--prefix=#{install_dir}/embedded"]

  if mac_os_x?
    python_configure.push("--enable-ipv6",
                          "--with-universal-archs=intel",
                          "--enable-shared",
                          "--without-gcc",
                          "CC=clang",
                          "MACOSX_DEPLOYMENT_TARGET=10.12")
  elsif linux?
    python_configure.push("--enable-unicode=ucs4")
  end

  build do
    ship_license "PSFL"

    patch :source => "avoid-allocating-thunks-in-ctypes.patch" if linux?
    patch :source => "fix-platform-ubuntu.diff" if linux?

    command python_configure.join(" "), :env => env
    command "make -j #{workers}", :env => env
    command "make install", :env => env
    delete "#{install_dir}/embedded/lib/python2.7/test"

    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/lib-dynload/readline.*"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/lib-dynload/gdbm.so"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/lib-dynload/dbm.so"))
    end
  end

else
  default_version "2.7.16"

  dependency "vc_redist"
  source :url => "https://s3.amazonaws.com/dd-agent-omnibus/python-windows-#{version}-nopip-amd64.zip",
         :sha256 => "528c33b78be915731f3cb9e6e1f9328abd58e43911ea7941e3e4057a2ec130df",
         :extract => :seven_zip

  build do
    #
    # expand python zip into the embedded directory
    command "XCOPY /YEHIR *.* \"#{windows_safe_path(python_2_embedded)}\""
  end
end
