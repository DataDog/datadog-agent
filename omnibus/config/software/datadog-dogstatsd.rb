# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require 'pathname'

name 'datadog-dogstatsd'

skip_transitive_dependency_licensing true

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }

  unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
    gomodcache = Pathname.new(ENV["OMNIBUS_GOMODCACHE"])
    env["GOMODCACHE"] = gomodcache.to_path
  end

  if windows_target?
    major_version_arg = "%MAJOR_VERSION%"
  else
    major_version_arg = "$MAJOR_VERSION"
  end

  # we assume the go deps are already installed before running omnibus
  command "invoke dogstatsd.build --rebuild --major-version #{major_version_arg}", env: env

  mkdir "#{install_dir}/etc/datadog-dogstatsd"
  unless windows_target?
    mkdir "#{install_dir}/run/"
    mkdir "#{install_dir}/scripts/"
  end

  # move around bin and config files
  if windows_target?
    mkdir "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"
    copy 'bin/dogstatsd/dogstatsd.exe', "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"
  else
    copy 'bin/dogstatsd/dogstatsd', "#{install_dir}/bin"
  end
  move 'bin/dogstatsd/dist/dogstatsd.yaml', "#{install_dir}/etc/datadog-dogstatsd/dogstatsd.yaml.example"

  if linux_target?
    if debian_target?
      erb source: "upstart_debian.conf.erb",
          dest: "#{install_dir}/scripts/datadog-dogstatsd.conf",
          mode: 0644,
          vars: { install_dir: install_dir }
    # Ship a different upstart job definition on RHEL to accommodate the old
    # version of upstart (0.6.5) that RHEL 6 provides.
    elsif redhat_target? || suse_target?
      erb source: "upstart_redhat.conf.erb",
          dest: "#{install_dir}/scripts/datadog-dogstatsd.conf",
          mode: 0644,
          vars: { install_dir: install_dir }
    end
    erb source: "systemd.service.erb",
        dest: "#{install_dir}/scripts/datadog-dogstatsd.service",
        mode: 0644,
        vars: { install_dir: install_dir }
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
