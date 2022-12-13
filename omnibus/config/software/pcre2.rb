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

name "pcre2"
default_version "10.41"

license "LGPLv2"
skip_transitive_dependency_licensing true

version '10.41' do
  source url: 'https://github.com/PCRE2Project/pcre2/releases/download/pcre2-10.41/pcre2-10.41.tar.bz2',
         sha512: '328f331a56f152424f6021b37f8dcf660842c55d43ff39f1b49115f0d05ed651d0bbb66b43c0ed61d65022030615768b92ce5e6218a54e4e17152ec473cca68d'
end

version '10.37' do
  source url: 'https://github.com/PCRE2Project/pcre2/releases/download/pcre2-10.37/pcre2-10.37.tar.bz2',
         sha512: '69f4bf4736b986e0fc855eedb292efe72a0df2e803bc0e61a6cf47775eed433bb1b2f28d7e641591ef4603d47beb543a64ed0eef9538d00f0746bc3435c143ec'
end

version '10.10' do
  source url: 'https://github.com/PCRE2Project/pcre2/releases/download/pcre2-10.10/pcre2-10.10.tar.bz2',
         sha512: 'c012022793cb6e569009590e12aee3ce847064fe09358fe98da9d67f4d150b798a6a92d54b2df31a352a21e79a098aac9ea801d7fa8d37cdcc77b6d0d6bdb5a7'
end

relative_path "pcre2-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_command = [
    "./configure",
    "--prefix=#{install_dir}/embedded",
  ]

  command configure_command.join(" "), env: env

  make "-j #{workers}", env: env
  make "-j #{workers} install", env: env
end
