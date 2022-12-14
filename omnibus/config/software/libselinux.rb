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

name "libselinux"
default_version "3.0"

dependency 'pcre2'
dependency 'libsepol'

license "LGPLv2"
skip_transitive_dependency_licensing true

version '3.4' do
  source url: 'https://github.com/SELinuxProject/selinux/releases/download/3.4/libselinux-3.4.tar.gz',
         sha512: '7ffa6d2159d2333d836bde3f75dfc78a278283b66ae1e441c178371adb6f463aa6f2d62439079e2068d1135c39dd2b367b001d917c0bdc6871a73630919ef81e'
end

version '3.0' do
  source url: 'https://github.com/SELinuxProject/selinux/releases/download/20191204/libselinux-3.0.tar.gz',
         sha512: '6fd8c3711e25cb1363232e484268609b71d823975537b3863e403836222eba026abce8ca198f64dba6f4c1ea4deb7ecef68a0397b9656a67b363e4d74409cd95'
end

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "ln_no_relative.patch", env: env

  make "-j #{workers} PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
  make "-j #{workers} install PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
end
