# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'datadog-puppy'

license "Apache-2.0"
license_file "../LICENSE"

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  etc_dir = "/etc/datadog-agent"
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }
  # include embedded path (mostly for `pkg-config` binary)
  env = with_embedded_path(env)

  if windows?
    major_version_arg = "%MAJOR_VERSION%"
    py_runtimes_arg = "%PY_RUNTIMES%"
  else
    major_version_arg = "$MAJOR_VERSION"
    py_runtimes_arg = "$PY_RUNTIMES"
  end

  if linux?
    command "invoke agent.build --puppy --rebuild --no-development --python-runtimes #{py_runtimes_arg} --major-version #{major_version_arg}", env: env
    copy('bin', install_dir)

    mkdir "#{install_dir}/run/"

  
    # Config
    mkdir '/etc/datadog-agent'
    mkdir "/var/log/datadog"

    move 'bin/agent/dist/datadog.yaml', '/etc/datadog-agent/datadog.yaml.example'
    move 'bin/agent/dist/conf.d', '/etc/datadog-agent/'

    if debian?
      erb source: "upstart.conf.erb",
          dest: "/etc/init/datadog-agent.conf",
          mode: 0644,
          vars: { install_dir: install_dir }

      erb source: "systemd.service.erb",
          dest: "/lib/systemd/system/datadog-agent.service",
          mode: 0644,
          vars: { install_dir: install_dir }
    end
  end
  if windows?
    platform = windows_arch_i386? ? "x86" : "x64"

    conf_dir = "#{install_dir}/etc/datadog-agent"
    mkdir conf_dir
    mkdir "#{install_dir}/bin/agent"

    command "inv agent.build --puppy --rebuild --no-development --arch #{platform} --python-runtimes #{py_runtimes_arg} --major-version #{major_version_arg}", env: env

      # move around bin and config files
    move 'bin/agent/dist/datadog.yaml', "#{conf_dir}/datadog.yaml.example"
    #move 'bin/agent/dist/system-probe.yaml', "#{conf_dir}/system-probe.yaml.example"
    move 'bin/agent/dist/conf.d', "#{conf_dir}/"
    copy 'bin/agent', "#{install_dir}/bin/"

    command "invoke customaction.build --major-version #{major_version_arg} --arch=" + platform

    # Build the process-agent with the correct go version for windows
    command "invoke -e process-agent.build --major-version #{major_version_arg} --arch #{platform}", :env => env

    copy 'bin/process-agent/process-agent.exe', "#{Omnibus::Config.source_dir()}/datadog-puppy/src/github.com/DataDog/datadog-agent/bin/agent"
  

  end
  block do
    if windows?
      # defer compilation step in a block to allow getting the project's build version, which is populated
      # only once the software that the project takes its version from (i.e. `datadog-agent`) has finished building
      env['TRACE_AGENT_VERSION'] = project.build_version.gsub(/[^0-9\.]/, '') # used by gorake.rb in the trace-agent, only keep digits and dots
      platform = windows_arch_i386? ? "x86" : "x64"
      command "invoke trace-agent.build --major-version #{major_version_arg} --arch #{platform}", :env => env

      copy 'bin/trace-agent/trace-agent.exe', "#{Omnibus::Config.source_dir()}/datadog-puppy/src/github.com/DataDog/datadog-agent/bin/agent"
    end
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
