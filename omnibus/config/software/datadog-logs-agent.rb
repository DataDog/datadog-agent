# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

require 'pathname'

name "datadog-logs-agent"
always_build true

binary_name = "logs"
binary = "bin/logs/#{binary_name}"
log_agent_binary_name = "logs-agent"
log_agent_binary = "#{install_dir}/bin/agent/#{log_agent_binary_name}"

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }
  command "invoke logs.build", env: env
  command "chmod +x #{binary}"
  move binary, log_agent_binary
end
