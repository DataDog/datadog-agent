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
  default_version "2.7.18"

  dependency "libxcrypt"
  dependency "ncurses"
  dependency "zlib"
  dependency "openssl"
  dependency "pkg-config"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "libyaml"

  source :url => "http://python.org/ftp/python/#{version}/Python-#{version}.tgz",
         :sha256 => "da3080e3b488f648a3d7a4560ddee895284c3380b11d6de75edb986526b9a814"

  relative_path "Python-#{version}"

  env = {
    "CFLAGS" => "-I#{install_dir}/embedded/include -O2 -g -pipe -fPIC",
    "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
    "PKG_CONFIG" => "#{install_dir}/embedded/bin/pkg-config",
    "PKG_CONFIG_PATH" => "#{install_dir}/embedded/lib/pkgconfig"
  }

  python_configure = ["./configure",
                      "--prefix=#{install_dir}/embedded",
                      "--with-ensurepip=no"] # pip is installed separately by its own software def

  if mac_os_x?
    python_configure.push("--enable-ipv6",
                          "--with-universal-archs=intel",
                          "--enable-shared",
                          "--disable-static",
                          "--without-gcc",
                          "CC=clang")
  elsif linux?
    python_configure.push("--enable-unicode=ucs4",
                          "--enable-shared",
                          "--disable-static")
  end

  build do
    # 2.0 is the license version here, not the python version
    license "Python-2.0"

    patch :source => "avoid-allocating-thunks-in-ctypes.patch" if linux?
    patch :source => "fix-platform-ubuntu.diff" if linux?
    # security patches backported by the debian community
    # see: http://deb.debian.org/debian/pool/main/p/python2.7/python2.7_2.7.18-6.diff.gz
    patch :source => "python2.7_2.7.18-cve-2019-20907.diff" unless windows?
    patch :source => "python2.7_2.7.18-cve-2020-8492.diff" unless windows?
    patch :source => "python2.7_2.7.18-cve-2021-3177.diff" unless windows?

    command python_configure.join(" "), :env => env
    command "make -j #{workers}", :env => env
    command "make install", :env => env
    delete "#{install_dir}/embedded/lib/python2.7/test"

    move "#{install_dir}/embedded/bin/2to3", "#{install_dir}/embedded/bin/2to3-2.7"

    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/lib-dynload/readline.*"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/lib-dynload/gdbm.so"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/lib-dynload/dbm.so"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/distutils/command/wininst-*.exe"))
    end
  end

else
  default_version "2.7.18-8829519"
  dependency "vc_redist"

  if windows_arch_i386?
    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-x86.zip",
           :sha256 => "295F16FB166AC26624AE9CBA08666DB437E0B8DDBB8D8D987F0598B71E4B6B24".downcase,
           :extract => :seven_zip
  else
    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-x64.zip",
         :sha256 => "58424EEB272E5678E732402CAF150124CD583B81F5DA442C911CE71A63ECD339".downcase,
         :extract => :seven_zip
  end
  build do
    # 2.0 is the license version here, not the python version
    license "Python-2.0"

    #
    # expand python zip into the embedded directory
    command "XCOPY /YEHIR *.* \"#{windows_safe_path(python_2_embedded)}\""
  end
end
