#
# Copyright:: Copyright (c) 2012-2014 Chef Software, Inc.
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

name "ncurses"
default_version "6.4-20230527"

dependency "libgcc"
dependency "libtool" if ohai["platform"] == "aix"
dependency "config_guess"

# Original binaries at https://invisible-island.net/archives/ncurses/current/
# Cached on S3 as invisible-island.net blocks default Ruby http User-Agent request header
source url: "https://s3.amazonaws.com/dd-agent-omnibus/ncurses-#{version}.tgz",
       sha256: "ded8c3b05c3af64b11b019fb2e07f41150a604208e0b6f07cce9ca7ebba54931",
       extract: :seven_zip

relative_path "ncurses-#{version}"

env = with_embedded_path
env = with_standard_compiler_flags(env, aix: { use_gcc: true })

build do
  license "MIT"
  license_file "https://gist.githubusercontent.com/remh/41a4f7433c77841c302c/raw/d15db09a192ca0e51022005bfb4c3a414a996896/ncurse.LICENSE"

  env.delete("CPPFLAGS")

  update_config_guess

  # build wide-character libraries
  configure_options = [
    "--with-shared",
    "--disable-static",
    "--with-termlib",
    "--without-debug",
    "--without-normal", # AIX doesn't like building static libs
    "--without-cxx-binding",
    "--enable-overwrite",
    "--enable-widec",
    "--enable-pc-files",
    "--without-manpages",
    "--without-tests",
    "--without-progs",
    "--disable-db-install",
  ]

  configure_options << "--with-libtool" if ohai["platform"] == "aix"
  configure(*configure_options, env: env)
  command "make -j #{workers}", env: env
  command "make -j #{workers} install", env: env

  # Python's detection seems to expect to fiind a ncurses.pc rather than ncursesw.pc
  link "#{install_dir}/embedded/lib/pkgconfig/ncursesw.pc", "#{install_dir}/embedded/lib/pkgconfig/ncurses.pc"

  # Ensure embedded ncurses wins in the LD search path
  if ohai["platform"] == "smartos"
    link "#{install_dir}/embedded/lib/libcurses.so", "#{install_dir}/embedded/lib/libcurses.so.1"
  end
end
