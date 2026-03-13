# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Custom tarball packager for creating uncompressed tar archives.
# This is useful for local development builds where compression time
# is not worth the disk space savings.

require "pathname"

module Omnibus
  class Packager::Tarball < Packager::Base
    id :tarball

    setup do
    end

    build do
      out_file = windows_safe_path(Config.package_dir, package_name)
      input_paths = ["#{windows_safe_path(project.install_dir)}/*"] + project.extra_package_files
      cmd = <<-EOH.split.join(" ").squeeze(" ").strip
        tar -cf
        #{out_file}
        #{input_paths.join(" ")}
      EOH
      shellout!(cmd)

      if debug_build?
        out_file = windows_safe_path(Config.package_dir, package_name(true))
        cmd = <<-EOH.split.join(" ").squeeze(" ").strip
          tar -cf
          #{out_file}
          #{debug_package_paths.map { |dir| File.join(install_dir, dir) }.join(' ')}
        EOH
        shellout!(cmd)
      end
    end

    # @see Base#package_name
    def package_name(debug = false)
      "#{project.package_name}#{debug ? "-dbg" : ""}-#{project.build_version}-#{project.build_iteration}-#{safe_architecture}.tar"
    end

    def safe_architecture
      raw = shellout!("uname --processor").stdout.strip

      case raw
      when "x86_64", "x64", "amd64" then "amd64"
      when "arm64", "aarch64" then "arm64"
      when "armv7l" then "arm"
      else raise ArgumentError, "Unknown architecture '#{raw}'"
      end
    end
  end

  # Register Tarball packager for all Linux platforms
  # The skip_packager logic in the project files controls when it's actually used
  Packager::PLATFORM_PACKAGER_MAP.each do |platform, packagers|
    next unless packagers.is_a?(Array)
    next if packagers.include?(Packager::Tarball)
    # Add Tarball to platforms that have XZ
    packagers << Packager::Tarball if packagers.include?(Packager::XZ)
  end
end
