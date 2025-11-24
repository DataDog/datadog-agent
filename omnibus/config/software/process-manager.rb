# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'

name 'process-manager'

# Source from local agent-process-manager directory in the repository
source path: '..'
relative_path 'agent-process-manager'

# Always rebuild to ensure we have the latest version
always_build true

build do
  license "Apache-2.0"
  license_file "./LICENSE"

  # Set up Rust/Cargo environment
  # Use the system's Rust installation
  # Optionally cache cargo dependencies for faster rebuilds
  env = with_standard_compiler_flags({})

  # Set CARGO_HOME for dependency caching (optional, improves rebuild performance)
  cargo_cache = "#{Omnibus::Config.cache_dir}/cargo"
  mkdir cargo_cache
  env['CARGO_HOME'] = cargo_cache

  # Build the process-manager in release mode
  # This produces two binaries: dd-procmgrd (daemon) and dd-procmgr (CLI)
  command "cargo build --release",
    env: env,
    cwd: project_dir,
    :live_stream => Omnibus.logger.live_stream(:info)

  # Copy both binaries to embedded/bin
  copy "#{project_dir}/target/release/dd-procmgrd", "#{install_dir}/embedded/bin/dd-procmgrd"
  copy "#{project_dir}/target/release/dd-procmgr", "#{install_dir}/embedded/bin/dd-procmgr"

  # Set appropriate permissions
  command "chmod 0755 #{install_dir}/embedded/bin/dd-procmgrd"
  command "chmod 0755 #{install_dir}/embedded/bin/dd-procmgr"

  # Create process-manager config directory
  pm_config_dir = "#{install_dir}/etc/pm"
  mkdir pm_config_dir

  # Copy DataDog agent process-manager configuration from agent repository
  # This file defines how the process manager should manage the agent process
  # The config file is in omnibus/config/files/pm/processes.yaml
  config_source = "#{Omnibus::Config.project_root}/config/files/pm/processes.yaml"
  copy config_source, "#{pm_config_dir}/processes.yaml"
end
