# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.
require 'pathname'

name 'datadog-dogstatsd'

license "Apache-2.0"
license_file "LICENSE"
skip_transitive_dependency_licensing true

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
  command "invoke dogstatsd.build --rebuild --major-version $MAJOR_VERSION", env: env

  mkdir "#{install_dir}/etc/datadog-dogstatsd"
  unless windows?
    mkdir "#{install_dir}/run/"
    mkdir "#{install_dir}/scripts/"
  end

  # move around bin and config files
  copy 'bin/dogstatsd/dogstatsd', "#{install_dir}/bin"
  move 'bin/dogstatsd/dist/dogstatsd.yaml', "#{install_dir}/etc/datadog-dogstatsd/dogstatsd.yaml.example"

  if linux?
    erb source: "upstart.conf.erb",
        dest: "#{install_dir}/scripts/datadog-dogstatsd.conf",
        mode: 0644,
        vars: { install_dir: install_dir }

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
