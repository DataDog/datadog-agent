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

name "preparation"
description "the steps required to preprare the build"
default_version "1.0.0"

build do
  block do
    %w{embedded/lib embedded/bin bin}.each do |dir|
      dir_fullpath = File.expand_path(File.join(install_dir, dir))
      FileUtils.mkdir_p(dir_fullpath)
      FileUtils.touch(File.join(dir_fullpath, ".gitkeep"))
    end
  end
end
