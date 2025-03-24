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

name "nfsiostat"
default_version "2.1.1"

source url: "https://mirrors.edge.kernel.org/pub/linux/utils/nfs-utils/#{version}/nfs-utils-#{version}.tar.gz",
       sha256: "381bb3f6aa4b314538db0bcfb242da855c2eb36e2059cf61aa498c0220684363",
       extract: :seven_zip

relative_path "nfs-utils-#{version}"

build do
  license "GPL-2.0"

  mkdir "#{install_dir}/embedded/sbin/"
  command "sed -i 's:#!/usr/bin/python:#!/opt/datadog-agent/embedded/bin/python:' ./tools/nfs-iostat/nfs-iostat.py"
  command "cp ./tools/nfs-iostat/nfs-iostat.py #{install_dir}/embedded/sbin/nfsiostat"
  command "chmod +x #{install_dir}/embedded/sbin/nfsiostat"
end
