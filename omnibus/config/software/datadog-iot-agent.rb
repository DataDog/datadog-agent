# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'datadog-iot-agent'

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

  if windows_target?
    major_version_arg = "%MAJOR_VERSION%"
  else
    major_version_arg = "$MAJOR_VERSION"
    env['CGO_CFLAGS'] = "-I#{install_dir}/embedded/include"
  end

  if linux_target?
    command "invoke agent.build --flavor iot --no-development --major-version #{major_version_arg}", env: env
    mkdir "#{install_dir}/bin"
    mkdir "#{install_dir}/run/"


    # Config
    mkdir '/etc/datadog-agent'
    mkdir "/etc/init"
    mkdir "/var/log/datadog"

    move 'bin/agent/dist/datadog.yaml', '/etc/datadog-agent/datadog.yaml.example'
    move 'bin/agent/dist/conf.d', '/etc/datadog-agent/'
    copy 'bin/agent', "#{install_dir}/bin/"
  end
  block do
    if windows_target?
      # just builds the trace-agent, this should be moved to a separate package as it's not related to the iot agent
      platform = windows_arch_i386? ? "x86" : "x64"
      command "invoke trace-agent.build --major-version #{major_version_arg}", :env => env

      mkdir "#{install_dir}/bin/agent"
      copy 'bin/trace-agent/trace-agent.exe', "#{Omnibus::Config.source_dir()}/datadog-iot-agent/src/github.com/DataDog/datadog-agent/bin/agent"
    end
  end
end
