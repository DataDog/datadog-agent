#
# Copyright:: Copyright (c) 2014 Chef Software, Inc.
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

# A requirement for api.berkshelf.com that is used in berkshelf specs
# https://github.com/berkshelf/api.berkshelf.com

name "libarchive"
default_version "3.1.2"

source url: "https://www.libarchive.org/downloads/libarchive-#{version}.tar.gz",
       sha256: "eb87eacd8fe49e8d90c8fdc189813023ccc319c5e752b01fb6ad0cc7b2c53d5e"

relative_path "libarchive-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)
  configure_options = [
    "--without-lzma",
    "--without-lzo2",
    "--without-nettle",
    "--without-xml2",
    "--without-expat",
    "--without-bz2lib",
    "--without-iconv",
    "--without-zlib",
    "--disable-bsdtar",
    "--disable-bsdcpio",
    "--without-lzmadec",
    "--without-openssl",
  ]
  configure(*configure_options, env: env)
  command "make -j #{workers}", env: env
  command "make install", env: env
end
