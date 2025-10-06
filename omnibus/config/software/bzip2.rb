#
# Copyright 2013-2018 Chef Software, Inc.
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
# Install bzip2 and its shared library, libbz2.so
# This library object is required for building Python with the bz2 module,
# and should be picked up automatically when building Python.

name "bzip2"
default_version "1.0.8"

license "BSD-2-Clause"
license_file "LICENSE"
skip_transitive_dependency_licensing true

dependency "zlib"

# version_list: url=https://sourceware.org/pub/bzip2/ filter=*.tar.gz
version("1.0.8") { source sha256: "ab5a03176ee106d3f0fa90e381da478ddae405918153cca248e682cd0c4a2269" }

source url: "https://fossies.org/linux/misc/#{name}-#{version}.tar.gz"

build do
  command "bazelisk run -- @bzip2//:install --destdir='#{install_dir}/embedded'", \
    cwd: "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent"

  # This is temporary until we fix pkg_install to deal with symlinks
  link "libbz2.so.1.0.8", "#{install_dir}/embedded/lib/libbz2.so.1.0"
  link "libbz2.so.1.0.8", "#{install_dir}/embedded/lib/libbz2.so.1"
  link "libbz2.so.1.0.8", "#{install_dir}/embedded/lib/libbz2.so"

  # The version of bzip2 we use doesn't create a pkgconfig file,
  # we add it here manually (needed at least by the Python build)
  erb source: "bzip2.pc.erb",
      dest: "#{install_dir}/embedded/lib/pkgconfig/bzip2.pc",
      vars: {
        prefix: File.join(install_dir, "embedded"),
        version: version,
      }

end
