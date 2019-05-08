# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'stackstate-puppy'

license "Apache-2.0"
license_file "../LICENSE"

source path: '..'
relative_path 'src/github.com/StackVista/stackstate-agent'

build do
  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  etc_dir = "/etc/stackstate-agent"
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }
  # include embedded path (mostly for `pkg-config` binary)
  env = with_embedded_path(env)

  command "invoke agent.build --puppy --rebuild --no-development", env: env
  copy('bin', install_dir)

  mkdir "#{install_dir}/run/"

  if linux?
    # Config
    mkdir '/etc/stackstate-agent'
    mkdir "/var/log/stackstate-agent"

    move 'bin/agent/dist/stackstate.yaml', '/etc/stackstate-agent/stackstate.yaml.example'
    move 'bin/agent/dist/conf.d', '/etc/stackstate-agent/'

    if debian?
      erb source: "upstart.conf.erb",
          dest: "/etc/init/stackstate-agent6.conf",
          mode: 0644,
          vars: { install_dir: install_dir }

      erb source: "systemd.service.erb",
          dest: "/lib/systemd/system/stackstate-agent6.service",
          mode: 0644,
          vars: { install_dir: install_dir }
    end
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
