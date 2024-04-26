# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require 'pathname'

name 'datadog-agentless-scanner'

skip_transitive_dependency_licensing true

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  etc_dir = "/etc/datadog-agent"
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }

  unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
    gomodcache = Pathname.new(ENV["OMNIBUS_GOMODCACHE"])
    env["GOMODCACHE"] = gomodcache.to_path
  end

  # we assume the go deps are already installed before running omnibus
  command "invoke agentless-scanner.build --rebuild --major-version $MAJOR_VERSION", env: env

  mkdir "#{install_dir}/etc/datadog-agent"
  mkdir "#{install_dir}/run/"
  mkdir "#{install_dir}/scripts/"

  # move around bin and config files
  copy 'bin/agentless-scanner/agentless-scanner', "#{install_dir}/bin"

  if debian_target?
    erb source: "upstart_debian.conf.erb",
        dest: "#{install_dir}/scripts/datadog-agentless-scanner.conf",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    erb source: "sysvinit_debian.agentless-scanner.erb",
        dest: "#{install_dir}/scripts/datadog-agentless-scanner",
        mode: 0755,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
  elsif redhat_target? || suse_target?
    # Ship a different upstart job definition on RHEL to accommodate the old
    # version of upstart (0.6.5) that RHEL 6 provides.
    erb source: "upstart_redhat.conf.erb",
        dest: "#{install_dir}/scripts/datadog-agentless-scanner.conf",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
  end
  erb source: "systemd.service.erb",
      dest: "#{install_dir}/scripts/datadog-agentless-scanner.service",
      mode: 0644,
      vars: { install_dir: install_dir, etc_dir: etc_dir }

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
