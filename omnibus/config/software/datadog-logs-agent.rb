# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

name "datadog-logs-agent"
always_build true

build do
  logs_agent_version = "alpha"
  binary = "logagent"
  url = "https://s3.amazonaws.com/public.binaries.sheepdog.datad0g.com/agent/#{logs_agent_version}/linux-amd64/#{binary}"
  command "curl -f #{url} -o #{binary}"
  command "chmod +x #{binary}"
  command "mv #{binary} #{install_dir}/bin/agent/logs-agent"
end
