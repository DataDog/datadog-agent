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
default_version "1.0.9"

source :url => "https://github.com/gluster/gstatus/releases/download/v#{version}/gstatus",
       :sha256 => "4731a515ce1b75c7d9f378588be5be369ca0130fb51802de853527310724b0f8",
       :target_filename => "gstatus"


build do
  license "GPL-3.0"

  # fix the shebang of the gstatus artifact to use the embedded agent python environment
  # #!/usr/bin/env python3 -> #!/opt/datadog-agent/embedded/bin/python
  command "sed -i '1s|.*|#!#{install_dir}/embedded/bin/python|' gstatus"
  copy "gstatus", "#{install_dir}/embedded/sbin/gstatus"
  command "chmod +x #{install_dir}/embedded/sbin/gstatus"
end
