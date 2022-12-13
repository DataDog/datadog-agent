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
default_version "3.0"

license "LGPLv2"
skip_transitive_dependency_licensing true

version '3.4' do
  source url: 'https://github.com/SELinuxProject/selinux/releases/download/3.4/libsepol-3.4.tar.gz',
         sha512: '7ffa6d2159d2333d836bde3f75dfc78a278283b66ae1e441c178371adb6f463aa6f2d62439079e2068d1135c39dd2b367b001d917c0bdc6871a73630919ef81e'
end

version '3.0' do
  source url: 'https://github.com/SELinuxProject/selinux/releases/download/20191204/libsepol-3.0.tar.gz',
         sha512: '82a5bae0afd9ae53b55ddcfc9f6dd61724a55e45aef1d9cd0122d1814adf2abe63c816a7ac63b64b401f5c67acb910dd8e0574eec546bed04da7842ab6c3bb55'
end

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  patch source: "ln_no_relative.patch", env: env

  make "-j #{workers} PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
  make "-j #{workers} install PREFIX=/ DESTDIR=#{install_dir}/embedded", env: env
end
