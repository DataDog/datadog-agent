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

license "Apache License Version 2.0"
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

  # we assume the go deps are already installed before running omnibus
  command "invoke agent.build --rebuild --use-embedded-libs --no-development", env: env

  mkdir "#{install_dir}/etc/datadog-agent"
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
  move 'bin/agent/dist/datadog.yaml', "#{install_dir}/etc/datadog-agent/datadog.yaml.example"

  move 'bin/agent/dist/trace-agent.conf', "#{install_dir}/etc/datadog-agent/"
  move 'bin/agent/dist/process-agent.conf', "#{install_dir}/etc/datadog-agent/"

  if windows?
    move 'bin/agent/dist/conf.d', "#{install_dir}/etc/datadog-agent/"
  end

  if linux?
    if debian?
      erb source: "upstart_debian.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent.conf",
          mode: 0755,
          vars: { install_dir: install_dir }
    elsif redhat?
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


  delete "#{install_dir}/uselessfile"
end
