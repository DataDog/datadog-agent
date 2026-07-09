# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'

name 'ai-usage-agent'

source path: '..',
       options: {
         exclude: [
           "**/.cache/**/*",
           "**/testdata/**/*",
         ],
       }
relative_path 'src/github.com/DataDog/datadog-agent'

always_build true

build do
  license :project_license

  # The AI Usage Chrome native messaging host is a Rust binary built by Bazel. It ships inside the
  # "eudm" fleet installer extension layer (Windows only), so we stage the flat layout the
  # extension hook expects — the binary and the example config at the install_dir root:
  #   <install_dir>/ai-usage-agent-native-host.exe
  #   <install_dir>/ai_usage_native_host.yaml.example
  # (see pkg/fleet/installer/packages/datadog_agent_eudm_windows.go).
  if windows_target?
    command "bazel run -- //cmd/ai_prompt_logger:install-extension --destdir=#{install_dir}", :live_stream => Omnibus.logger.live_stream(:info)
  end
end
