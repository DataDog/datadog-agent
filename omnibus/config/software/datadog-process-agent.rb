# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

name "datadog-process-agent"
always_build true
require "./lib/ostools.rb"

process_agent_version = ENV['PROCESS_AGENT_VERSION']
if process_agent_version.nil? || process_agent_version.empty?
  process_agent_version = 'master'
end
process_agent_branch = ENV['PROCESS_AGENT_BRANCH']
if process_agent_branch.nil? || process_agent_branch.empty?
  process_agent_branch = 'master'
end
default_version process_agent_version

build do
  if windows?
    binary = "stackstate-process-agent-#{version}.exe"
    target_binary = "process-agent.exe"
    url = "https://stackstate-process-agent-2-test.s3.amazonaws.com/#{binary}"
    curl_cmd = "powershell -Command wget -OutFile #{binary} #{url}"
    command curl_cmd
    command "mv #{binary} #{install_dir}/bin/agent/#{target_binary}"
  else
    binary = "process-agent-amd64-#{process_agent_version}"
    target_binary = "process-agent"
    url = "https://s3.amazonaws.com/stackstate-process-agent-test/binaries/#{process_agent_branch}/#{binary}"
    curl_cmd = "curl -f #{url} -o #{binary}"
    command curl_cmd
    command "chmod +x #{binary}"
    command "mv #{binary} #{install_dir}/embedded/bin/#{target_binary}"
  end
end
