# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'installer'

source path: '..',
       options: {
         exclude: ["**/.cache/**/*", "**/testdata/**/*"],
       }
relative_path 'src/github.com/DataDog/datadog-agent'

always_build true

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

  if linux_target?
    command "invoke installer.build --no-cgo --run-path=/opt/datadog-packages/run --install-path=#{install_dir}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    mkdir "#{install_dir}/bin"
    copy 'bin/installer', "#{install_dir}/bin/"

    # Always build the package
    command_on_repo_root "bazelisk build --config=release //packages/installer/linux:whole_distro_tar_deb", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    # There are no convenience symlinks, so we need to do some path manipulations to get the absolute path.
    command_on_repo_root "bazelisk cquery --config=release --output=files //packages/installer/linux:whole_distro_tar_deb | sed -e 's@bazel-out/@@' >/tmp/installer_linux_tar_deb_file.txt"
    command_on_repo_root "tar tvf $(bazelisk info output_path)/$(cat /tmp/installer_linux_tar_deb_file.txt)", :live_stream => Omnibus.logger.live_stream(:info)
    if debian_target?
      command_on_repo_root "bazelisk build --config=release //packages/installer/linux:debian"
    elsif redhat_target? || suse_target?
      command_on_repo_root "bazelisk build --config=release //packages/installer/linux:rpm"
    end
  elsif windows_target?
    command "dda inv -- -e installer.build --install-path=#{install_dir}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    copy 'bin/installer/installer.exe', "#{install_dir}/datadog-installer.exe"
  end
end
