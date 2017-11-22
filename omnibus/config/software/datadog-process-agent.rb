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
  binary_url = "https://s3.amazonaws.com/datad0g-process-agent/#{binary}"

  # fetch the binary and move to install_dir
  command "curl -f #{binary_url} -o #{binary}"
  command "chmod +x #{binary}"
  move binary, "#{install_dir}/embedded/bin/process-agent"
end
