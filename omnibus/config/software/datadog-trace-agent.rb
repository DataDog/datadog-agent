# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

require "./lib/ostools.rb"
require 'pathname'

name "datadog-trace-agent"

dependency "datadog-agent"

default_version "master"

source git: 'https://github.com/DataDog/datadog-trace-agent.git'
relative_path 'src/github.com/DataDog/datadog-trace-agent'

if windows?
  trace_agent_binary = "trace-agent.exe"
else
  trace_agent_binary = "trace-agent"
end

build do
  ship_license "https://raw.githubusercontent.com/DataDog/datadog-trace-agent/#{version}/LICENSE"

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  env = {
   'GOPATH' => gopath.to_path,
   'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }

  command "go get github.com/Masterminds/glide", :env => env
  command "glide install", :env => env
  command "go generate ./info", :env => env

  block do
    # defer compilation step in a block to allow getting the project's build version, which is populated
    # only once the software that the project takes its version from (i.e. `datadog-agent`) has finished building
    agent_version = project.build_version.gsub(/[^0-9\.]/, '') # used by "go generate ./info" in the trace-agent, only keep digits and dots
    ver_array = agent_version.split(".")
    env['TRACE_AGENT_VERSION'] = agent_version
    if windows?
      command "windmc --target pe-x86-64 -r cmd/trace-agent/windows_resources cmd/trace-agent/windows_resources/trace-agent-msg.mc", :env => env
      command "windres --define MAJ_VER=#{ver_array[0]} --define MIN_VER=#{ver_array[1]} --define PATCH_VER=#{ver_array[2]} -i cmd/trace-agent/windows_resources/trace-agent.rc --target=pe-x86-64 -O coff -o cmd/trace-agent/rsrc.syso", :env => env
      command "go build -a ./cmd/...", :env => env
      copy trace_agent_binary, "#{install_dir}/bin/agent"
    else
      command "go build -a ./cmd/...", :env => env
      copy trace_agent_binary, "#{install_dir}/embedded/bin"
    end
  end
end
