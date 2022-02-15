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

name "gstatus"
default_version "1.0.5"

source :url => "https://github.com/gluster/gstatus/releases/download/v#{version}/gstatus",
       :sha256 => "485b79c42d5623e2593374be3b8d8cde8a00f080ab2fe417c84a2dc3d2a49719",
       :target_filename => "gstatus"


build do
  license "GPL-3.0"

  copy "gstatus", "#{install_dir}/embedded/sbin/gstatus"
  command "chmod +x #{install_dir}/embedded/sbin/gstatus"
end
