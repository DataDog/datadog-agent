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
default_version "3.7.7"

source url: "https://www.libarchive.org/downloads/libarchive-#{version}.tar.xz",
       sha256: "879acd83c3399c7caaee73fe5f7418e06087ab2aaf40af3e99b9e29beb29faee"

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
    "--disable-static",
  ]
  configure(*configure_options, env: env)
  command "make -j #{workers}", env: env
  command "make install", env: env
end
