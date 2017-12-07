# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

require "./lib/ostools.rb"
require 'pathname'

name "datadog-logs-agent-win"

default_version "db/windows_svc"

source git: 'https://github.com/DataDog/datadog-agent.git'
relative_path 'src/github.com/DataDog/datadog-agent'

if windows?
  log_agent_binary = "logs.exe"
else
  log_agent_binary = "logs"
end

build do
  ship_license "https://raw.githubusercontent.com/DataDog/datadog-agent/#{version}/LICENSE"

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  if windows?
    env = {
        'GOPATH' => gopath.to_path,
        'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
        'TRACE_AGENT_VERSION' => default_version, # used by gorake.rb in the trace-agent
        'WINDRES' => 'true',
    }
  else
    env = {
        'GOPATH' => gopath.to_path,
        'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
        'TRACE_AGENT_VERSION' => default_version, # used by gorake.rb in the trace-agent
    }
  end

  #command "go get github.com/Masterminds/glide", :env => env
  command "inv deps", :env => env
  command "inv logs.build", :env => env
  if windows?
    copy "#{install_dir}/bin/logs/#{log_agent_binary}", "#{install_dir}/bin/agent"
  else
    copy log_agent_binary, "#{install_dir}/embedded/bin"
  end
end
