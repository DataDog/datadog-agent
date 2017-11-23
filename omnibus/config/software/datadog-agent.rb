# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

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
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }
  # include embedded path (mostly for `pkg-config` binary)
  env = with_embedded_path(env)

  # we assume the go deps are already installed before running omnibus
  command "invoke agent.build --rebuild --use-embedded-libs --no-development", env: env

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

  # move around bin and config files
  copy 'bin', install_dir
  move 'bin/agent/dist/datadog.yaml', "#{conf_dir}/datadog.yaml.example"

  move 'bin/agent/dist/trace-agent.conf', "#{conf_dir}/"
  move 'bin/agent/dist/process-agent.conf', "#{conf_dir}/"

  if windows?
    move 'bin/agent/dist/conf.d', "#{conf_dir}/"
  end

  if linux?
    if debian?
      erb source: "upstart_debian.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent.conf",
          mode: 0755,
          vars: { install_dir: install_dir }
    elsif redhat? || suse?
      # Ship a different upstart job definition on RHEL to accommodate the old
      # version of upstart (0.6.5) that RHEL 6 provides.
      erb source: "upstart_redhat.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent.conf",
          mode: 0755,
          vars: { install_dir: install_dir }
    end

    erb source: "systemd.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent.service",
        mode: 0755,
        vars: { install_dir: install_dir }
  end

  if osx?
    # conf
    copy 'cmd/agent/com.datadoghq.agent.plist.example', "#{install_dir}/etc/"

    # Systray GUI
    # app_temp_dir = "#{install_dir}/Datadog Agent.app/Contents"
    # mkdir "#{app_temp_dir}/Resources"
    # copy 'packaging/osx/app/Agent.icns', "#{app_temp_dir}/Resources/"

    # mkdir "#{app_temp_dir}/MacOS"
    # command 'cd packaging/osx/gui && swiftc -O -target "x86_64-apple-macosx10.10" -static-stdlib Sources/*.swift -o gui && cd ../../..'
    # copy "packaging/osx/gui/gui", "#{app_temp_dir}/MacOS/"
    # copy "packaging/osx/gui/Sources/agent.png", "#{app_temp_dir}/MacOS/"
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  unless windows?
    delete "#{install_dir}/uselessfile"
  end
end
