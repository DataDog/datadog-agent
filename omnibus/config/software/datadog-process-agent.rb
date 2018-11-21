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
default_version process_agent_version

url = "https://s3.amazonaws.com/datad0g-process-agent/"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/datadog-process-agent/#{version}/LICENSE"
  if windows?
    binary = "process-agent-windows-#{version}.exe"
    target_binary = "process-agent.exe"
    curl_cmd = "powershell -Command wget -OutFile #{binary} #{url}#{binary}"
    command curl_cmd
    command "mv #{binary} #{install_dir}/bin/agent/#{target_binary}"
  else
    binary = "process-agent-amd64-#{version}"
    target_binary = "process-agent"
    curl_cmd = "curl -f #{url}#{binary} -o #{binary}"
    command curl_cmd
    command "chmod +x #{binary}"
    command "mv #{binary} #{install_dir}/embedded/bin/#{target_binary}"

    # network-tracer versions will always be the same on both process-agent and network-tracer
    ship_license "https://raw.githubusercontent.com/DataDog/tcptracer-bpf/#{version}/LICENSE.network-tracer"
    binary = "network-tracer-amd64-#{version}"
    target_binary = "network-tracer"
    curl_cmd = "curl -f #{url}#{binary} -o #{binary}"
    command curl_cmd
    command "chmod +x #{binary}"
    command "mv #{binary} #{install_dir}/embedded/bin/#{target_binary}"
  end
end
