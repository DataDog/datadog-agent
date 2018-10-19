# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

require "./lib/ostools.rb"
require 'pathname'

name "datadog-process-agent"

dependency "datadog-agent"

process_agent_version = ENV['PROCESS_AGENT_VERSION']
if process_agent_version.nil? || process_agent_version.empty?
  process_agent_version = 'master'
end
default_version process_agent_version

source git: 'https://github.com/DataDog/datadog-process-agent.git'
relative_path 'src/github.com/DataDog/datadog-process-agent'

if windows?
  process_agent_binary = "process-agent.exe"
else
  process_agent_binary = "process-agent"
end

build do
  ship_license "https://raw.githubusercontent.com/DataDog/datadog-process-agent/#{version}/LICENSE"
  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  if windows?
    env = {
      # Trace agent uses GNU make to build.  Some of the input to gnu make
      # needs the path with `\` as separators, some needs `/`.  Provide both,
      # and let the makefile sort it out (ugh)

      # also on windows don't modify the path.  Modifying the path here mixes
      # `/` with `\` in the PATH variable, which confuses the make (and sub-processes)
      # below.  When properly configured the path on the windows box is sufficient.
      'GOPATH' => "#{windows_safe_path(gopath.to_path)}",
    }
  else
    env = {
      'GOPATH' => gopath.to_path,
      'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
    }
  end

  block do
    # defer compilation step in a block to allow getting the project's build version, which is populated
    # only once the software that the project takes its version from (i.e. `datadog-agent`) has finished building
    env['PROCESS_AGENT_VERSION'] = project.build_version.gsub(/[^0-9\.]/, '') # used by gorake.rb in the process-agent, only keep digits and dots

    # build process-agent
    command "rake deps", :env => env
    command "rake build", :env => env

    # copy binary
    if windows?
      copy "#{project_dir}/#{process_agent_binary}", "#{install_dir}/bin/agent"
    else
      copy "#{project_dir}/#{process_agent_binary}", "#{install_dir}/embedded/bin"
    end
  end
end
