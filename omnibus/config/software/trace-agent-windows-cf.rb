# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'trace-agent-windows-cf'
description 'Builds trace-agent.exe for the Windows Cloud Foundry buildpack'

source path: '..',
       options: {
         exclude: ["**/.cache/**/*", "**/.git/fsmonitor--daemon.ipc"],
       }
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

  gopath = Pathname.new(project_dir) + '../../../..'
  env = with_embedded_path()
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => ["#{gopath.to_path}/bin", env['PATH']].join(File::PATH_SEPARATOR),
  }

  unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
    env["GOMODCACHE"] = ENV["OMNIBUS_GOMODCACHE"]
  end

  command "invoke trace-agent.build", :env => env, :live_stream => Omnibus.logger.live_stream(:info)

  source_bin_dir = "#{Omnibus::Config.source_dir()}/trace-agent-windows-cf/src/github.com/DataDog/datadog-agent/bin/agent"
  mkdir source_bin_dir
  copy 'bin/trace-agent/trace-agent.exe', "#{source_bin_dir}/trace-agent.exe"
  copy 'bin/trace-agent/trace-agent.exe.pdb', "#{source_bin_dir}/trace-agent.exe.pdb"
end
