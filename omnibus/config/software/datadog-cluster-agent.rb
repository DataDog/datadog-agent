# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'datadog-cluster-agent'

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
  command "invoke cluster-agent.build --rebuild --use-embedded-libs", env: env

  if osx?
    conf_dir = "#{install_dir}/etc"
  else
    conf_dir = "#{install_dir}/etc/datadog-cluster-agent"
  end
  mkdir conf_dir
  unless windows?
    mkdir "#{install_dir}/run/"
    mkdir "#{install_dir}/scripts/"
  end

  # move around bin and config files
  move 'bin/datadog-cluster-agent/dist/datadog-cluster.yaml', "#{conf_dir}/datadog-cluster.yaml"
  move 'bin/datadog-cluster-agent/dist/conf.d', "#{conf_dir}/"
  copy 'bin', install_dir

  if linux?
    erb source: "upstart.conf.erb",
        dest: "#{install_dir}/scripts/datadog-cluster-agent.conf",
        mode: 0755,
        vars: { install_dir: install_dir }

    erb source: "systemd.service.erb",
        dest: "#{install_dir}/scripts/datadog-cluster-agent.service",
        mode: 0755,
        vars: { install_dir: install_dir }
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  unless windows?
    delete "#{install_dir}/uselessfile"
  end
end
