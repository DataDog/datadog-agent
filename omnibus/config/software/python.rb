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

# This software definition is identical to the omnibus-software one,
# it was copied here to enable the `-fPIC` cflag on linux.
# TODO: Consolidate with the omnibus-software definition as soon as the
# change has been tested on Agent 5 as well.
name "python"

if ohai["platform"] != "windows"
  default_version "2.7.15"

  dependency "ncurses"
  dependency "zlib"
  dependency "openssl"
  dependency "bzip2"
  dependency "libsqlite3"

  source :url => "http://python.org/ftp/python/#{version}/Python-#{version}.tgz",
         :sha256 => "18617d1f15a380a919d517630a9cd85ce17ea602f9bbdc58ddc672df4b0239db"

  relative_path "Python-#{version}"

  env = {
    "CFLAGS" => "-I#{install_dir}/embedded/include -O2 -g -pipe",
    "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
  }

  if linux?
    # Emit position-independent code (Agent can't be forced to build and be healthy with --no-pie,
    # and allows building the Agent on ARM)
    env["CFLAGS"] += " -fPIC"
  end

  python_configure = ["./configure",
                      "--enable-universalsdk=/",
                      "--prefix=#{install_dir}/embedded"]

  if mac_os_x?
    python_configure.push("--enable-ipv6",
                          "--with-universal-archs=intel",
                          "--enable-shared")
  elsif linux?
    python_configure.push("--enable-unicode=ucs4")
  end

  python_configure.push("--with-dbmliborder=")

  build do
    ship_license "PSFL"
    patch :source => "python-2.7.11-avoid-allocating-thunks-in-ctypes.patch" if linux?
    patch :source => "python-2.7.11-fix-platform-ubuntu.diff" if linux?

    command python_configure.join(" "), :env => env
    command "make -j #{workers}", :env => env
    command "make install", :env => env
    delete "#{install_dir}/embedded/lib/python2.7/test"

    # There exists no configure flag to tell Python to not compile readline support :(
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/lib-dynload/readline.*"))
    end
  end

else
  default_version "2.7.15"

  dependency "vc_redist"
  source :url => "https://s3.amazonaws.com/dd-agent-omnibus/python-windows-#{version}-amd64.zip",
         :sha256 => "e3b099206e61b1b4bf70b89c5fe5a698ba2ac465133c6b00ed998227a19c4b83",
         :extract => :seven_zip

  build do
    #
    # expand python zip into the embedded directory
    command "XCOPY /YEHIR *.* \"#{windows_safe_path(install_dir)}\\embedded\""
  end
end
