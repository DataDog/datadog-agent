module ProjectExtensions
  def build
    packager = packagers_for_system[0]

    # Install any package this project extends
    extended_packages.each do |packages, enablerepo|
      log.info(log_key) { "installing #{packages}" }
      packager.install(packages, enablerepo)
    end

    super

    # Remove any package this project extends, after the health check ran
    extended_packages.each do |packages, _|
      log.info(log_key) { "removing #{packages}" }
      packager.remove(packages)
    end

    def package_me
      Stripper.run!(self) if strip_build
      super
      # If we also generate a debug package, copy it back into the workspace
      if packager.debug_build?
        debug_package_path = File.join(Config.package_dir, packager.package_name(true))
        FileUtils.cp(debug_package_path, destination, preserve: true)
      end
    end
  end
end

module Omnibus
  class Project
    prepend ProjectExtensions

    def sources_dir(val = NULL)
      if null?(val)
        @sources_dir || File.expand_path("#{files_path}/sources")
      else
        @sources_dir = val.tr('\\', "/").squeeze("/").chomp("/")
      end
    end

    expose :sources_dir

    def python_2_embedded(val = NULL)
      if null?(val)
        @python_2_embedded || "#{install_dir}/embedded2"
      else
        @python_2_embedded = val.tr('\\', "/").squeeze("/").chomp("/")
      end
    end

    expose :python_2_embedded

    def python_3_embedded(val = NULL)
      if null?(val)
        @python_3_embedded || "#{install_dir}/embedded3"
      else
        @python_3_embedded = val.tr('\\', "/").squeeze("/").chomp("/")
      end
    end

    expose :python_3_embedded

    #
    # Add a debug package path.
    #
    # Paths added here will be excluded from the main package and added to
    # the debug variant instead.
    #
    # @example
    #   debug_path 'foo/bar'
    #   dependency 'quz'
    #
    # @return [Array<String>]
    #   the list of dependencies
    #
    def debug_path(pattern)
      debug_package_paths << pattern
      debug_package_paths.dup
    end

    expose :debug_path

    #
    # Add a strip exclude path.
    #
    # Paths added here will be excluded from the stripping process.
    #
    # @example
    #   strip_exclude 'foo/bar'
    #   dependency 'quz'
    #
    # @param [String] val
    #   the path to exclude from stripping
    #
    # @return [Array<String>]
    #   the list of dependencies
    #
    def strip_exclude(pattern)
      strip_exclude_paths << pattern
      strip_exclude_paths.dup
    end

    expose :strip_exclude

    #
    # Add a package that is a recommended runtime dependency of this project.
    #
    # @example
    #   runtime_recommended_dependency 'foo'
    #
    # @param [String] val
    #   the name of the recommended runtime dependency
    #
    # @return [Array<String>]
    #   the list of recommended runtime dependencies
    #
    def runtime_recommended_dependency(val)
      runtime_recommended_dependencies << val
      runtime_recommended_dependencies.dup
    end

    expose :runtime_recommended_dependency

    # Add package(s) that this project extends.
    #
    # Use this to avoid packaging many files and libraries already included by
    # the extended projects.
    # This means that the project will rely on the extended packages to be
    # installed to behave as expected.
    # Extending a project is similar to running `apt-get install packages` before the
    # project build, and `apt-get purge packages` before the packaging
    #

    # @example
    #   extends_packages 'datadog-agent dd-check-mysql' 'datadog'
    #
    # @param [String] packages
    #   the name of the extended packages
    # @param [String] enablerepo
    #   set if a specific repository needs to be enabled (`--enablerepo` for rpm)
    #
    # @return [Array<String>]
    #   the list of extended packages
    #
    def extends_packages(packages, enablerepo = NULL)
      extended_packages << [packages, enablerepo]
      extended_packages.dup
    end

    expose :extends_packages

    #
    # Add one file in Windows build whose symbol need to be stripped
    #
    # @example
    #   windows_symbol_stripping_file "C:\\omnibus-ruby\\datadog-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\security-agent.exe"
    #
    # @param [String] val
    #   the name of the file in Windows build to be stripped
    #
    # @return [Array<String>]
    #   the list of files to be stripped
    #
    def windows_symbol_stripping_file(val)
      windows_symbol_stripping_files << val
      windows_symbol_stripping_files.dup
    end

    expose :windows_symbol_stripping_file

    #
    # The list of files in Windows build whose symbol need to be stripped.
    #
    # @return [Array<String>]
    #
    def windows_symbol_stripping_files(val = NULL)
      @windows_symbol_stripping_files ||= []
    end

    #
    # Method to enable whether or not a build should be stripped.
    #
    # @example
    #   strip_build = true
    #
    # @return [String]
    #
    def strip_build(val = NULL)
      if null?(val)
        @strip_build || false
      else
        @strip_build = val
      end
    end

    expose :strip_build

    #
    # Set or retrieve additional {#third_party_licenses} of the project.
    #
    # @example
    #   third_party_licenses 'LICENSES/third-party.csv'
    #
    # @param [String] val
    #   the location to the CSV file with the additional third party license list.
    #   The expected format to the CSV is as follows:
    #
    #   Component,Origin,License
    #   core,"github.com/DataDog/foo/bar",BSD-3-Clause
    #
    # @return [String]
    #
    def third_party_licenses(val = NULL)
      if null?(val)
        @third_party_licenses || "Unspecified"
      else
        @third_party_licenses = val
      end
    end

    expose :third_party_licenses

    #
    # Add all sources that have to be shipped to the project's
    # {install_dir}/sources.
    #
    # @return [true]
    #
    def install_sources
      log.info(log_key) { "Searching for sources to ship with the package." }
      if Dir.exist?(sources_dir)
        log.info(log_key) { "Sources found in #{sources_dir}. Moving them to #{install_dir}/sources." }
        FileUtils.mkdir_p("#{install_dir}/sources")
        FileUtils.cp_r("#{sources_dir}/.", "#{install_dir}/sources")
      else
        log.info(log_key) { "No sources found." }
      end

      true
    end

    # The list of paths to include in the debug package.
    # Paths here specified will be excluded from the main build.
    #
    # @see #debug_path
    #
    # @param [Array<String>]
    #
    # @return [Array<String>]
    #
    def debug_package_paths
      @debug_package_paths ||= []
    end

    # The list of paths to exclude in the stripping process.
    # Paths here specified will be excluded when stripping.
    #
    # @see #strip_exclude
    #
    # @param [Array<String>]
    #
    # @return [Array<String>]
    #
    def strip_exclude_paths
      @strip_exclude_paths ||= []
    end

    #
    # The list of recommended software dependencies for this project.
    #
    # @return [Array<String>]
    #
    def runtime_recommended_dependencies
      @runtime_recommended_dependencies ||= []
    end

    # The list of packages this project extends.
    #
    # @return [Array<String>]
    #
    def extended_packages
      @extended_packages ||= []
    end

    #
    # [MacOS only]
    # Set or return the code signing identity. Can be used in software definitions
    # to sign components of the package.
    #
    # @example
    #   code_signing_identity "foo"
    #
    # @param [String] val
    #   the identity to use when code signing
    #
    # @return [String]
    #   the code-signing identity
    #
    def code_signing_identity(val = NULL)
      if null?(val)
        @code_signing_identity
      else
        @code_signing_identity = val
      end
    end

    expose :code_signing_identity

    #
    # [MacOS only]
    # Set or return the location of the Entitlements file. Can be used
    # in software definitions to specify entitlements when signing files.
    #
    # @example
    #   entitlements_file "foo"
    #
    # @param [String] val
    #   the location of the Entitlements file
    #
    # @return [String]
    #   the location of the Entitlements file
    #
    def entitlements_file(val = NULL)
      if null?(val)
        @entitlements_file
      else
        @entitlements_file = val
      end
    end

    expose :entitlements_file

  end
end
