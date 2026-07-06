# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require './lib/fips.rb'
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

  # Build the FIPS flavor of the installer when AGENT_FLAVOR=fips, mirroring
  # datadog-agent.rb. Without this the standalone datadog-installer package ships
  # a non-FIPS binary, so its BuiltForFIPS() is false and it would download the
  # non-FIPS package variant even in a FIPS deployment.
  fips_args = fips_mode? ? "--fips-mode" : ""
  if fips_mode?
    # FIPS requires the Microsoft Go toolchain; the build tags are silently
    # ignored by the default compiler.
    add_msgo_to_env(env)
  end

  bazel_flags = "--//:install_dir=#{install_dir}"

  if linux_target?
    command "invoke installer.build #{fips_args} --no-cgo --run-path=/opt/datadog-packages/run --install-path=#{install_dir}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    mkdir "#{install_dir}/bin"
    copy 'bin/installer', "#{install_dir}/bin/"

    if fips_mode?
      # Prove the FIPS build tags took effect (OpenSSL is linked), as the build
      # succeeding is not a sufficient guarantee. See lib/fips.rb.
      block do
        fips_check_binary_for_expected_symbol(File.join(install_dir, "bin", "installer", "installer"))
      end

      # The FIPS installer binary (requirefips) uses a $ORIGIN-relative RPATH to
      # find libcrypto at runtime. When it is exec'd from a temp directory by the
      # FIPS agent daemon (which has CapabilityBoundingSet=all in its systemd unit),
      # the kernel sets AT_SECURE on the exec and the dynamic linker silently drops
      # $ORIGIN expansion. The binary falls through to the ldconfig cache and loads
      # the host's system libcrypto which may be a different version from the
      # embedded one, causing the requirefips self-test to fail with a panic.
      #
      # Add /opt/datadog-agent/embedded/lib as an absolute RPATH entry. Absolute
      # entries are always honoured regardless of AT_SECURE. On any host with
      # datadog-fips-agent installed this path contains the correct version-matched
      # libcrypto. The agent's own embedded lib is always the right version to use
      # because the daemon passes OPENSSL_CONF and OPENSSL_MODULES from the same
      # tree (fipsEnvFromRunningInstaller), so all three are version-consistent.
      command "patchelf --add-rpath /opt/datadog-agent/embedded/lib #{install_dir}/bin/installer/installer"
    end

    # Build both packages and dump them where gitlab will upload them.
    command "bazel build #{bazel_flags} //packages/installer/linux:whole_distro_tar_deb", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    # There are no convenience symlinks, so we need to do some path manipulations to get the absolute path.
    command "bazel cquery #{bazel_flags} --output=files //packages/installer/linux:whole_distro_tar_deb | sed -e 's@bazel-out/@@' >/tmp/installer_linux_tar_deb_file.txt"
    command "tar tvf $(bazel info output_path)/$(cat /tmp/installer_linux_tar_deb_file.txt)", :live_stream => Omnibus.logger.live_stream(:info)

    # Copy both the .deb and .rpm out to artifact outputs
    # In the package job, we'll compare these to the omnibus built packages and report the diffs
    # When the diffs go away, we delete the package job and just use this one.
    if ENV["OMNIBUS_PACKAGE_DIR"]
      omnibus_package_dir = ENV["OMNIBUS_PACKAGE_DIR"]
    elsif ENV["CI_PROJECT_DIR"]
      ci_project_dir = ENV["CI_PROJECT_DIR"]
      omnibus_package_dir = "#{ci_project_dir}/omnibus/pkg"
    end
    if omnibus_package_dir
      command "bazel run #{bazel_flags} -- //packages/installer/linux:copy_out --destdir=#{omnibus_package_dir}",
        :live_stream => Omnibus.logger.live_stream(:info)
    end
  elsif windows_target?
    command "dda inv -- -e installer.build #{fips_args} --install-path=#{install_dir}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    copy 'bin/installer/installer.exe', "#{install_dir}/datadog-installer.exe"
    copy 'bin/installer/installer.exe.pdb', "#{install_dir}/datadog-installer.exe.pdb"
  end
end
