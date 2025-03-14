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

#
# NOTE: Instead of depending on this software definition, there is no
#       reason not to include "-static-libgcc" in your LDFLAGS instead.
#       That will probably be the best solution going forwards rather than
#       fuss around with the dynamic linking business here.
#
name "libgcc"
description "On UNIX systems where we bootstrap a compiler, copy the libgcc"
default_version "0.0.1"

libgcc_file =
  case ohai["platform"]
  when "solaris2"
    "/opt/csw/lib/libgcc_s.so.1"
  when "aix"
    "/opt/freeware/lib/pthread/ppc64/libgcc_s.a"
  when "freebsd"
    "/lib/libgcc_s.so.1"
  else
    nil
  end

build do
  license "GPL-3.0"

  if libgcc_file
    if File.exist?(libgcc_file)
      copy "#{libgcc_file}", "#{install_dir}/embedded/lib/"
    else
      raise "cannot find libgcc -- where is your gcc compiler?"
    end
  else
    # If there's nothing to do (on OSX for instance), we still create a file
    # in datadog agent to be sure there's something to commit.
    # Indeed, since libgcc is often (always?) the first piece of software to
    # be built, it may trigger a bug when using git cache : git tries to tag
    # the /opt/datadog-agent repo after this build but can't find a valid
    # HEAD since no commit could have been done.
    command "touch #{install_dir}/uselessfile"
  end
end
