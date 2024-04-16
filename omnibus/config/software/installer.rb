# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'installer'

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  etc_dir = "/etc/datadog-agent"
  gomodcache = Pathname.new("/modcache")
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }

  unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
    gomodcache = Pathname.new(ENV["OMNIBUS_GOMODCACHE"])
    env["GOMODCACHE"] = gomodcache.to_path
  end

  # include embedded path (mostly for `pkg-config` binary)
  env = with_embedded_path(env)

  if linux_target?
    command "invoke installer.build --rebuild --install-path=#{install_dir}", env: env
    mkdir "#{install_dir}/bin"
    mkdir "#{install_dir}/run/"
    mkdir "#{install_dir}/systemd/"


    # Config
    mkdir "/etc/datadog-agent"
    mkdir "/etc/init"
    mkdir "/var/log/datadog"
    mkdir "/var/run/datadog-packages"

    # Packages
    mkdir "/opt/datadog-packages"
    copy 'bin/installer', "#{install_dir}/bin/"
    copy '#{files_path}/installer/Readme.md', "#{install_dir}/Readme.md"

    systemdPath = "#{install_dir}/systemd/"
    erb source: "datadog-installer.service.erb",
       dest: systemdPath + "datadog-installer.service",
       mode: 0644,
       vars: { installer_dir: "/opt/datadog-packages/datadog-installer/stable", etc_dir: etc_dir}

    erb source: "datadog-installer-exp.service.erb",
       dest: systemdPath + "datadog-installer-exp.service",
       mode: 0644,
       vars: { installer_dir: "/opt/datadog-packages/datadog-installer/experiment", etc_dir: etc_dir}

    # Add stable agent units
    templateToFile = {
      "datadog-agent.service.erb" => "datadog-agent.service",
      "datadog-agent-trace.service.erb" => "datadog-agent-trace.service",
      "datadog-agent-process.service.erb" => "datadog-agent-process.service",
      "datadog-agent-security.service.erb" => "datadog-agent-security.service",
      "datadog-agent-sysprobe.service.erb" => "datadog-agent-sysprobe.service",
    }
    templateToFile.each do |template, file|
      agent_dir = "/opt/datadog-packages/datadog-agent/stable"
      erb source: template,
         dest: systemdPath + file,
         mode: 0644,
         vars: { install_dir: install_dir, etc_dir: etc_dir, agent_dir: agent_dir }
    end
    # Add experiment agent units
    expTemplateToFile = {
      "datadog-agent-exp.service.erb" => "datadog-agent-exp.service",
      "datadog-agent-trace-exp.service.erb" => "datadog-agent-trace-exp.service",
      "datadog-agent-process-exp.service.erb" => "datadog-agent-process-exp.service",
      "datadog-agent-security-exp.service.erb" => "datadog-agent-security-exp.service",
      "datadog-agent-sysprobe-exp.service.erb" => "datadog-agent-sysprobe-exp.service",
    }
    expTemplateToFile.each do |template, file|
      agent_dir = "/opt/datadog-packages/datadog-agent/experiment"
      erb source: template,
         dest: systemdPath + file,
         mode: 0644,
         vars: { etc_dir: etc_dir, agent_dir: agent_dir }
    end

  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end

