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

name "version-manifest"
description "generates a version manifest file"
default_version "0.0.1"

skip_transitive_dependency_licensing true

build do
  license :project_license

  block do
    project_name = project.name
    project_build_version = project.build_version

    File.open("#{install_dir}/version-manifest.txt", "w") do |f|
      f.puts "#{project_name} #{project_build_version}"
      f.puts ""
      f.puts Omnibus::Reports.pretty_version_map(project)
    end
  end
end