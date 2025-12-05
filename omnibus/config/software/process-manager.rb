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

  # Process manager is available on Linux and Windows
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
    etc_dir = "/etc/datadog-agent/process-manager"
    mkdir "#{etc_dir}/processes.d"
    copy 'process_manager/examples/datadog-agent.yaml', "#{etc_dir}/processes.d/datadog-agent.yaml"
    copy 'process_manager/examples/datadog-agent-trace.yaml', "#{etc_dir}/processes.d/datadog-agent-trace.yaml"
    copy 'process_manager/examples/datadog-agent-trace.socket.yaml', "#{etc_dir}/processes.d/datadog-agent-trace.socket.yaml"
    copy 'process_manager/examples/datadog-agent-process.yaml', "#{etc_dir}/processes.d/datadog-agent-process.yaml"
    copy 'process_manager/examples/datadog-agent-security.yaml', "#{etc_dir}/processes.d/datadog-agent-security.yaml"
    copy 'process_manager/examples/datadog-agent-sysprobe.yaml', "#{etc_dir}/processes.d/datadog-agent-sysprobe.yaml"

    # Note: extra_package_file is registered at project level in agent.rb
  elsif windows_target?
    # Install MinGW-w64 if not present (required for GNU target)
    mingw_path = "C:\\mingw64\\bin"
    mingw_archive = "C:\\mingw64.7z"
    mingw_url = "https://github.com/niXman/mingw-builds-binaries/releases/download/14.2.0-rt_v12-rev1/x86_64-14.2.0-release-posix-seh-ucrt-rt_v12-rev1.7z"

    unless File.exist?("#{mingw_path}\\gcc.exe")
      command "powershell -Command \"[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; Invoke-WebRequest -Uri '#{mingw_url}' -OutFile '#{mingw_archive}' -UseBasicParsing\"", :live_stream => Omnibus.logger.live_stream(:info)
      command "7z x #{mingw_archive} -oC:\\ -y", :live_stream => Omnibus.logger.live_stream(:info)
    end

    # Install protoc if not present (required for prost-build)
    protoc_path = "C:\\protoc\\bin"
    protoc_archive = "C:\\protoc.zip"
    protoc_url = "https://github.com/protocolbuffers/protobuf/releases/download/v25.1/protoc-25.1-win64.zip"

    unless File.exist?("#{protoc_path}\\protoc.exe")
      command "powershell -Command \"[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; Invoke-WebRequest -Uri '#{protoc_url}' -OutFile '#{protoc_archive}' -UseBasicParsing\"", :live_stream => Omnibus.logger.live_stream(:info)
      command "powershell -Command \"Expand-Archive -Path '#{protoc_archive}' -DestinationPath 'C:\\protoc' -Force\"", :live_stream => Omnibus.logger.live_stream(:info)
    end

    # Add Rust GNU target
    command "rustup target add x86_64-pc-windows-gnu", :live_stream => Omnibus.logger.live_stream(:info)

    # Build the Rust binaries (daemon + CLI) for Windows using GNU toolchain
    env = {
      'PATH' => "#{protoc_path};#{mingw_path};#{ENV['USERPROFILE']}\\.cargo\\bin;#{ENV['PATH']}",
    }

    # Build with GNU target
    command "cd process_manager && cargo build --release --bins --target x86_64-pc-windows-gnu", env: env, :live_stream => Omnibus.logger.live_stream(:info)

    # Create necessary directories
    mkdir "#{install_dir}/bin/agent"

    # Copy both binaries to the install directory (Windows uses .exe extension)
    copy 'process_manager/target/x86_64-pc-windows-gnu/release/dd-procmgrd.exe', "#{install_dir}/bin/agent/dd-procmgrd.exe"
    copy 'process_manager/target/x86_64-pc-windows-gnu/release/dd-procmgr.exe', "#{install_dir}/bin/agent/dd-procmgr.exe"

    # Note: Windows config files are handled by the MSI installer
  end
end
