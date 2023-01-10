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

name "libacl"
default_version "2.3.1"

license "LGPLv2"
skip_transitive_dependency_licensing true

dependency 'attr'

version '2.3.1' do
  source url: 'http://download.savannah.nongnu.org/releases/acl/acl-2.3.1.tar.xz',
         sha512: '7d02f05d17305f8587ab485395b00c7fdb8e44c1906d0d04b70a43a3020803e8b2b8c707abb6147f794867dfa87bd51769c2d3e11a3db55ecbd2006a6e6231dc'
end

relative_path "acl-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_command = [
    "./configure",
    "--prefix=#{install_dir}/embedded",
    "--disable-static",
  ]

  command configure_command.join(" "), env: env

  make "-j #{workers}", env: env
  make "-j #{workers} install", env: env
end
