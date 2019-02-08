# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

require "./lib/ostools.rb"
require 'pathname'

name "stackstate-trace-agent"

dependency "datadog-agent"

trace_agent_version = ENV['TRACE_AGENT_VERSION']
if trace_agent_version.nil? || trace_agent_version.empty?
  trace_agent_version = 'master'
end

trace_agent_branch = ENV['TRACE_AGENT_BRANCH']
if trace_agent_branch.nil? || trace_agent_branch.empty?
  trace_agent_branch = 'master'
end

default_version trace_agent_version

source git: 'https://github.com/StackVista/stackstate-trace-agent.git'
relative_path 'src/github.com/StackVista/stackstate-trace-agent'

if windows?
  trace_agent_windows_binary = ENV['TRACE_AGENT_WINDOWS_BINARY']
  trace_agent_binary = "trace-agent.exe"
else
  trace_agent_binary = "trace-agent"
end

build do
  ship_license "https://raw.githubusercontent.com/StackVista/stackstate-trace-agent/#{version}/LICENSE"
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
    env['TRACE_AGENT_VERSION'] = project.build_version.gsub(/[^0-9\.]/, '') # used by gorake.rb in the trace-agent, only keep digits and dots

    # build trace-agent

    if windows?
      if trace_agent_windows_binary.nil? || trace_agent_windows_binary.empty?
          command "make windows", :env => env
      else
          binary = "stackstate-trace-agent-#{trace_agent_version}.exe"
          url = "https://s3-eu-west-1.amazonaws.com/stackstate-trace-agent-2-test/#{trace_agent_branch}/#{binary}"
          mkdir "#{gopath.to_path}/bin"
          curl_cmd = "powershell -Command wget -OutFile #{gopath.to_path}/bin/#{trace_agent_binary} #{url}"
          command curl_cmd
      end
    end
    command "make install", :env => env

    # copy binary
    if windows?
      copy "#{gopath.to_path}/bin/#{trace_agent_binary}", "#{install_dir}/bin/agent"
    else
      copy "#{gopath.to_path}/bin/#{trace_agent_binary}", "#{install_dir}/embedded/bin"
    end
  end
end
