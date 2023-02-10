#
# Copyright 2012-2019, Chef Software Inc.
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

name "libsepol"
default_version "3.4"

license "LGPLv2"
skip_transitive_dependency_licensing true

version '3.4' do
  source url: 'https://github.com/SELinuxProject/selinux/releases/download/3.4/libsepol-3.4.tar.gz',
         sha512: '5e47e6ac626f2bfc10a9f2f24c2e66c4d7f291ca778ebd81c7d565326e036e821d3eb92e5d7540517b1c715466232a7d7da895ab48811d037ad92d423ed934b6'
end

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "ln_no_relative.patch", env: env

  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"

  make "-j #{workers} PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
  make "-j #{workers} install PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
end
