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
  default_version "2.7.17"

  dependency "ncurses"
  dependency "zlib"
  dependency "openssl"
  dependency "pkg-config"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "libyaml"

  source :url => "http://python.org/ftp/python/#{version}/Python-#{version}.tgz",
         :sha256 => "f22059d09cdf9625e0a7284d24a13062044f5bf59d93a7f3382190dfa94cecde"

  relative_path "Python-#{version}"

  env = {
    "CFLAGS" => "-I#{install_dir}/embedded/include -O2 -g -pipe -fPIC",
    "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
    "PKG_CONFIG" => "#{install_dir}/embedded/bin/pkg-config",
    "PKG_CONFIG_PATH" => "#{install_dir}/embedded/lib/pkgconfig"
  }

  python_configure = ["./configure",
                      "--prefix=#{install_dir}/embedded"]

  if mac_os_x?
    python_configure.push("--enable-ipv6",
                          "--with-universal-archs=intel",
                          "--enable-shared",
                          "--without-gcc",
                          "CC=clang")
  elsif linux?
    python_configure.push("--enable-unicode=ucs4",
                          "--enable-shared")
  end

  build do
    ship_license "PSFL"

    patch :source => "avoid-allocating-thunks-in-ctypes.patch" if linux?
    patch :source => "fix-platform-ubuntu.diff" if linux?

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
  default_version "2.7.17"
  dependency "vc_redist"

  if windows_arch_i386?
    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-x86.zip",
           :sha256 => "a2c5c5736356f7264a7412cdca050cf2ae924c703c47a27b8f0d8f62ce2d6181",
           :extract => :seven_zip
  else
    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-amd64.zip",
         :sha256 => "557ea6690c5927360656c003d3114b73adbd755b712a2911975dde813d6d7afb",
         :extract => :seven_zip
  end
  build do
    #
    # expand python zip into the embedded directory
    command "XCOPY /YEHIR *.* \"#{windows_safe_path(python_2_embedded)}\""
  end
end
