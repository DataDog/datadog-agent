# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require 'pathname'

name 'process-manager'

skip_transitive_dependency_licensing true

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

  # Process manager is only available on Linux
  if linux_target?
    # Build the Rust binaries (daemon + CLI)
    env = {
      'PATH' => "#{ENV['HOME']}/.cargo/bin:#{ENV['PATH']}",
    }

    # Build both the daemon and CLI binaries using cargo workspace
    command "cd process_manager && cargo build --release --bins", env: env, :live_stream => Omnibus.logger.live_stream(:info)

    # Create necessary directories
    mkdir "#{install_dir}/bin"

    # Copy both binaries to the install directory
    copy 'process_manager/target/release/dd-procmgrd', "#{install_dir}/bin/dd-procmgrd"
    copy 'process_manager/target/release/dd-procmgr', "#{install_dir}/bin/dd-procmgr"

    # Create process manager config directory and copy config files
    etc_dir = "/etc/pm"
    mkdir "#{etc_dir}/processes.d"
    copy 'process_manager/examples/datadog-agent.yaml', "#{etc_dir}/processes.d/datadog-agent.yaml"
    copy 'process_manager/examples/datadog-agent-trace.yaml', "#{etc_dir}/processes.d/datadog-agent-trace.yaml"
    copy 'process_manager/examples/datadog-agent-trace.socket.yaml', "#{etc_dir}/processes.d/datadog-agent-trace.socket.yaml"
    copy 'process_manager/examples/datadog-agent-process.yaml', "#{etc_dir}/processes.d/datadog-agent-process.yaml"
    copy 'process_manager/examples/datadog-agent-security.yaml', "#{etc_dir}/processes.d/datadog-agent-security.yaml"
    copy 'process_manager/examples/datadog-agent-sysprobe.yaml', "#{etc_dir}/processes.d/datadog-agent-sysprobe.yaml"

    # Note: extra_package_file is registered at project level in agent.rb
  end
end

