# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'datadog-agent'

dependency 'python'
unless windows?
  dependency 'net-snmp-lib'
end

license "Apache-2.0"
license_file "../LICENSE"

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  etc_dir = "/etc/datadog-agent"
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }
  # include embedded path (mostly for `pkg-config` binary)
  env = with_embedded_path(env)

  # we assume the go deps are already installed before running omnibus
  command "invoke agent.build --rebuild --use-embedded-libs --no-development", env: env
  if windows?
    command "invoke systray.build --rebuild --use-embedded-libs --no-development", env: env
  end

  if osx?
    conf_dir = "#{install_dir}/etc"
  else
    conf_dir = "#{install_dir}/etc/datadog-agent"
  end
  mkdir conf_dir
  unless windows?
    mkdir "#{install_dir}/run/"
    mkdir "#{install_dir}/scripts/"
  end

  # if windows?
  #   mkdir "../../extra_package_files/EXAMPLECONFSLOCATION"
  #   copy "pkg/collector/dist/conf.d/*", "../../extra_package_files/EXAMPLECONFSLOCATION"
  # end

  ## build the custom action library required for the install
  if windows?
    command "invoke customaction.build"
  end

  # move around bin and config files
  move 'bin/agent/dist/datadog.yaml', "#{conf_dir}/datadog.yaml.example"
  move 'bin/agent/dist/network-tracer.yaml', "#{conf_dir}/network-tracer.yaml.example"
  move 'bin/agent/dist/conf.d', "#{conf_dir}/"

  copy 'bin', install_dir

  if linux?
    if debian?
      erb source: "upstart_debian.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.process.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-process.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.network.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-network.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.trace.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-trace.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.erb",
          dest: "#{install_dir}/scripts/datadog-agent",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.process.erb",
          dest: "#{install_dir}/scripts/datadog-agent-process",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.trace.erb",
          dest: "#{install_dir}/scripts/datadog-agent-trace",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
    elsif redhat? || suse?
      # Ship a different upstart job definition on RHEL to accommodate the old
      # version of upstart (0.6.5) that RHEL 6 provides.
      erb source: "upstart_redhat.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.process.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-process.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.network.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-network.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.trace.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-trace.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
    end

    erb source: "systemd.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent.service",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    erb source: "systemd.process.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent-process.service",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    erb source: "systemd.network.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent-network.service",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    erb source: "systemd.trace.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent-trace.service",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
  end

  if osx?
    # Launchd service definition
    erb source: "launchd.plist.example.erb",
        dest: "#{conf_dir}/com.datadoghq.agent.plist.example",
        mode: 0644,
        vars: { install_dir: install_dir }

    # Systray GUI
    app_temp_dir = "#{install_dir}/Datadog Agent.app/Contents"
    mkdir "#{app_temp_dir}/MacOS"
    systray_build_dir = "#{project_dir}/cmd/agent/gui/systray"
    # Target OSX 10.10 (it brings significant changes to Cocoa and Foundation APIs, and older versions of OSX are EOL'ed)
    command 'swiftc -O -swift-version "3" -target "x86_64-apple-macosx10.10" -static-stdlib Sources/*.swift -o gui', cwd: systray_build_dir
    copy "#{systray_build_dir}/gui", "#{app_temp_dir}/MacOS/"
    copy "#{systray_build_dir}/agent.png", "#{app_temp_dir}/MacOS/"
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  unless windows?
    delete "#{install_dir}/uselessfile"
  end
end
