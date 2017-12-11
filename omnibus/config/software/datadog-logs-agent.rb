# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

name "datadog-logs-agent"
always_build true

binary_name = "logs"
binary = "bin/logs/#{binary_name}"
log_agent_binary_name = "logs-agent"
log_agent_binary = "#{install_dir}/bin/agent/#{log_agent_binary_name}"

build do
  command "invoke logs.build"
  command "chmod +x #{binary}"
  move binary, log_agent_binary
end
