# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'datadog-iot-agent'

source path: '..',
       options: {
         exclude: ["**/.cache/**/*", "**/.git/fsmonitor--daemon.ipc"],
       }
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  etc_dir = "/etc/datadog-agent"
  gomodcache = Pathname.new("/modcache")
  # include embedded path (mostly for `pkg-config` binary)
  #
  # with_embedded_path prepends the embedded path to the PATH from the global environment
  # in particular it ignores the PATH from the environment given as argument
  # so we need to call it before setting the PATH
  env = with_embedded_path()
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => ["#{gopath.to_path}/bin", env['PATH']].join(File::PATH_SEPARATOR),
  }

  unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
    gomodcache = Pathname.new(ENV["OMNIBUS_GOMODCACHE"])
    env["GOMODCACHE"] = gomodcache.to_path
  end

  unless windows_target?
    env['CGO_CFLAGS'] = "-I#{install_dir}/embedded/include"
  end

  if linux_target?
    # Next steps:
    # - Add //cmd/installer:installer to the deps in //packages/agent/iot
    # - Drop the invoke here
    # - Drop the copy bin/agent -> install_dir/bin
    command "invoke agent.build --flavor iot --no-development", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    # Clean out the things that invoke agent.build leaves in bin/agent/dist, which we now get via bazel belowe.
    delete 'bin/agent/dist/conf.d'
    delete 'bin/agent/dist/datadog.yaml'

    # Installs: bin/ and run/ dirs
    command "bazel run --//packages/agent:flavor=iot --//:install_dir='#{install_dir}' -- " \
            "//packages/agent/iot:install --destdir=#{install_dir}", :live_stream => Omnibus.logger.live_stream(:info)
    copy 'bin/agent', "#{install_dir}/bin/"

    # Installs: example yaml
    command "bazel run --//packages/agent:flavor=iot --//:install_dir='#{install_dir}' -- " \
            "//packages/agent/iot:install_example_config --destdir=/", :live_stream => Omnibus.logger.live_stream(:info)

    # /var/log/datadog is a runtime directory; not managed by Bazel packaging.
    mkdir "/var/log/datadog"
  end
  block do
    if windows_target?
      # just builds the trace-agent, this should be moved to a separate package as it's not related to the iot agent

      command "invoke trace-agent.build", :env => env, :live_stream => Omnibus.logger.live_stream(:info)

      mkdir "#{Omnibus::Config.source_dir()}/datadog-iot-agent/src/github.com/DataDog/datadog-agent/bin/agent"
      copy 'bin/trace-agent/trace-agent.exe', "#{Omnibus::Config.source_dir()}/datadog-iot-agent/src/github.com/DataDog/datadog-agent/bin/agent/trace-agent.exe"
      copy 'bin/trace-agent/trace-agent.exe.pdb', "#{Omnibus::Config.source_dir()}/datadog-iot-agent/src/github.com/DataDog/datadog-agent/bin/agent/trace-agent.exe.pdb"
    end
  end
end
