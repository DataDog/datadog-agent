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

  env['CGO_CFLAGS'] = "-I#{install_dir}/embedded/include"

  if linux_target?
    # Temporary while we are still building with dda.
    # We need the systemd headers in place for to build coreos/go-systemd.
    # After migration we can delete this.
    command "bazel run #{omnibazel_flags} -- @systemd//:install --destdir=#{install_dir}", \
        :live_stream => Omnibus.logger.live_stream(:info)

    # Next steps:
    # - Add //cmd/installer:installer to the deps in //packages/agent/iot
    # - Drop the copy bin/agent -> install_dir/bin
    #
    # armhf cross-compiles from an aarch64 host via the linux_armv7 platform
    # (no native armv7 CI runners exist).
    platform_flags = ENV['PACKAGE_ARCH'] == 'armhf' ? ' --platforms=//bazel/platforms:linux_armv7' : ''
    command "bazel build#{platform_flags} //cmd/iot-agent", :live_stream => Omnibus.logger.live_stream(:info)
    mkdir 'bin/agent'
    command "cp \"$(bazel info execution_root)/$(bazel cquery#{platform_flags} //cmd/iot-agent " \
            "--output=starlark --starlark:expr='target.files.to_list()[0].path')\" " \
            "bin/agent/agent", :live_stream => Omnibus.logger.live_stream(:info)

    # Installs: bin/ and run/ dirs
    command "bazel run #{omnibazel_flags} -- " \
            "//packages/agent/iot:install --destdir=#{install_dir}", :live_stream => Omnibus.logger.live_stream(:info)
    copy 'bin/agent', "#{install_dir}/bin/"

    # Installs: example yaml
    command "bazel run #{omnibazel_flags} -- " \
            "//packages/agent/iot:install_example_config --destdir=/", :live_stream => Omnibus.logger.live_stream(:info)

    # /var/log/datadog is a runtime directory; not managed by Bazel packaging.
    mkdir "/var/log/datadog"
  end
end
