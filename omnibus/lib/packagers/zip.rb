# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require "pathname"
require "omnibus/packagers/windows_base"
require "fileutils"

module Omnibus
  class Packager::ZIP < Packager::WindowsBase
    id :zip

    setup do
    end

    build do
      if signing_identity
        puts "starting signing"
        if additional_sign_files
          additional_sign_files.each do |signfile|
            puts "signing #{signfile}"
            sign_package(signfile)
          end
        end

      end
      # If there are extra package files let's add them
      zip_file = windows_safe_path(Config.package_dir, zip_name)
      zip_source_path = "#{windows_safe_path(project.install_dir)}\\*"
      cmd = <<-EOH.split.join(" ").squeeze(" ").strip
        7z a -r
        #{zip_file}
        #{zip_source_path}
      EOH
      shellout!(cmd)

      if not extra_package_dirs.nil?
        extra_package_dirs.each do |extra_package_dir|
          if File.directory?(extra_package_dir)
            # Let's collect the DirectoryRefs
            zip_source_path = "#{windows_safe_path(extra_package_dir)}\\* "
            cmd = <<-EOH.split.join(" ").squeeze(" ").strip
              7z a -r
              #{zip_file}
              #{zip_source_path}
            EOH
            shellout!(cmd)
          end
        end
      end
    end

    #
    # @!group DSL methods
    # --------------------------------------------------

    #
    # set or retrieve additional files to sign
    #
    def additional_sign_files(val = NULL)
      if null?(val)
        @additional_sign_files
      else
        unless val.is_a?(Array)
          raise InvalidValue.new(:additional_sign_files, "be an Array")
        end

        @additional_sign_files = val
      end
    end
    expose :additional_sign_files

    def extra_package_dirs(val = NULL)
      if null?(val)
        @extra_package_dirs || nil
      else
        unless val.is_a?(Array)
          raise InvalidValue.new(:extra_package_dir, "be an Array")
        end

        @extra_package_dirs = val
      end
    end
    expose :extra_package_dirs

    #
    # @!endgroup
    # --------------------------------------------------

    # @see Base#package_name
    def package_name
      zip_name
    end

    # @see Base#debug_build?
    # The zip packager doesn't support debug packaging
    # HACK: This is needed to avoid failures when the Project#package_me method tries
    # to fetch the debug package produced by each packager,
    # as the Windows build uses both the MSI packager (which does have a debug package) and
    # the ZIP packager (which doesn't have a debug package).
    def debug_build?
      false
    end

    def zip_name
      "#{project.package_name}-#{project.build_version}-#{project.build_iteration}-#{Config.windows_arch}.zip"
    end
  end
end
