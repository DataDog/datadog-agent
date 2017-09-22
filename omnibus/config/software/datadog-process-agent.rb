# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

name "datadog-process-agent"
always_build true

default_version 'master'


build do
  ship_license "https://github.com/DataDog/datadog-process-agent/blob/#{version}/LICENSE"

  binary = "process-agent-amd64-#{version}"
  config = "dd-process-agent.ini.example"
  config_url = "https://raw.githubusercontent.com/DataDog/datadog-process-agent/#{version}/packaging/debian/package/etc/dd-agent/#{config}"
  binary_url = "https://s3.amazonaws.com/datad0g-process-agent/#{binary}"

  # fetch the binary and move to install_dir
  command "curl #{binary_url} -o #{binary}"
  command "chmod +x #{binary}"
  move binary, "#{install_dir}/embedded/bin/process-agent"

  # fetch the default config file, remove api_key settings and move to install_dir
  command "curl #{config_url} -o #{config}"
  command "sed '/^api_key/d' < #{config} > process-agent.ini" # works both on linux and mac
  move "process-agent.ini", "#{install_dir}/etc/datadog-agent"
end