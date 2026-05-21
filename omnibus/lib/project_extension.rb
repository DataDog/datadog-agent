require "find"
require "set"

require "./lib/symbols_inspectors"

module Omnibus
  module ProjectExtensions
    def install_sources
      # Unfortunately the stripper runs before the `package_me` method is called
      # so we have to override `install_sources` which runs before the stripper
      # to inspect non-stripped binaries
      if @inspectors
        @inspectors.each do|i|
          i.inspect()
        end
      end
      super
    end

    #
    # Add an inspection step for a binary.
    #
    # For now only supports Go binaries
    #
    # @example
    #   inspect_binary "#{Omnibus::Config.source_dir()}\\bin\\agent\\agent.exe" do |symbols|
    #
    #   end
    #
    def inspect_binary(binary_path, &block)
      @inspectors ||= []
      @inspectors.append(GoSymbolsInspector.new(binary_path, &block))
    end

    # Override the package_me step to sign the binaries just before the packagers run
    def package_me
      normalize_linux_package_permissions
      if @chmod_before_packaging
        @chmod_before_packaging.each do |file, mode|
          next unless File.exist?(file)

          File.chmod(mode, file)
        end
      end
      if @files_to_sign
        @files_to_sign.each do|file|
          ddwcssign(file)
        end
      end
      super
    end

    # Build images may make package output paths group-writable, setgid, and
    # owned by a shared build group so non-root builders can write to them.
    # Package managers record file metadata, so restore runtime ownership and
    # remove those build-only sharing bits before generating deb/rpm payloads.
    def normalize_linux_package_permissions
      return unless linux_target?

      normalize_package_path(install_dir)
      Array(extra_package_files).each do |path|
        normalize_package_path(path)
      end
    end

    def normalize_package_path(path)
      return unless File.exist?(path)

      normalize_path_tree_permissions(path)
      normalize_parent_permissions(path) if external_package_path?(path)
    end

    def normalize_path_tree_permissions(root)
      if File.directory?(root)
        Find.find(root) do |path|
          normalize_path_permissions(path)
        end
      else
        normalize_path_permissions(root)
      end
    end

    def normalize_parent_permissions(path)
      parent = File.dirname(File.expand_path(path))
      while parent != "/"
        normalize_path_permissions(parent)
        parent = File.dirname(parent)
      end
    end

    def external_package_path?(path)
      expanded_path = File.expand_path(path)
      project_root = File.expand_path(Omnibus::Config.project_root)
      install_root = File.expand_path(install_dir)

      !path_inside?(expanded_path, project_root) && !path_inside?(expanded_path, install_root)
    end

    def path_inside?(path, root)
      path == root || path.start_with?("#{root}/")
    end

    def normalize_path_permissions(path)
      return unless File.exist?(path)

      stat = File.lstat(path)
      return if stat.symlink?

      mode = stat.mode & 0o7777
      normalized_mode = stat.directory? ? mode & ~0o2020 : mode & ~0o020
      File.chmod(normalized_mode, path) if normalized_mode != mode
      File.chown(0, 0, path) if Process.euid == 0 && (stat.uid != 0 || stat.gid != 0)
    end

    def ddwcssign(file)
      log.info(self.class.name) { "Signing #{file}" }

      # Signing is inherently flaky as the timestamp server may not be available
      # retry a few times
      max_retries = 3
      attempts = 0
      delay = 2

      begin
        attempts += 1
        cmd = Array.new.tap do |arr|
          arr << "dd-wcs"
          arr << "sign"
          if ENV['WINDOWS_SIGNING_CERT'] && ENV['WINDOWS_SIGNING_CONFIG']
            arr << "--cert" << ENV['WINDOWS_SIGNING_CERT']
            arr << "--config" << ENV['WINDOWS_SIGNING_CONFIG']
          end
          arr << "\"#{file}\""
        end.join(" ")

        status = shellout(cmd)
        if status.exitstatus != 0
          log.warn(self.class.name) do
            <<-EOH.strip
              Failed to sign with dd-wcs (Attempt #{attempts} of #{max_retries})

              STDOUT
              ------
              #{status.stdout}

              STDERR
              ------
              #{status.stderr}
            EOH
          end
          raise "Failed to sign with dd-wcs"
        else
          log.info(self.class.name) { "Successfully signed #{file} after #{attempts} attempt(s)" }
        end
      rescue => e
        # Retry logic: raise error after 3 attempts
        if attempts < max_retries
          log.info(self.class.name) { "Retrying signing #{file} (Attempt #{attempts + 1})" }
          sleep(delay)
          retry
        end
        raise "Failed to sign with dd-wcs: #{e.message}"
      end
    end

    #
    # Add a signing step for a file.
    #
    def sign_file(file_path)
      @files_to_sign ||= []
      @files_to_sign.append(file_path)
    end

    #
    # Restore final permissions just before packaging.
    #
    def chmod_before_packaging(file_path, mode)
      @chmod_before_packaging ||= []
      @chmod_before_packaging.append([file_path, mode])
    end
  end

  class Project
    prepend ProjectExtensions
    expose :inspect_binary
    expose :sign_file
    expose :chmod_before_packaging
  end

  # Notarize and staple the .pkg after it is signed. Without this, Gatekeeper
  # rejects the .pkg when extracted from the .dmg (e.g. by Homebrew), because
  # the notarization ticket only covers the outer .dmg, not the .pkg inside it.
  module PackagerPKGNotarizer
    def run!
      super
      notarize_and_staple if signing_identity
    end

    private

    def notarize_and_staple
      pkg_path    = final_pkg
      apple_id    = ENV.fetch('APPLE_ACCOUNT')
      password    = ENV.fetch('NOTARIZATION_PWD')
      team_id     = ENV.fetch('TEAM_ID')
      timeout     = ENV.fetch('NOTARIZATION_TIMEOUT')
      max_retries = Integer(ENV.fetch('NOTARIZATION_ATTEMPTS'))

      log.info(self.class.name) { "Notarizing #{pkg_path}" }

      submission_id = nil
      max_retries.times do |attempt|
        status = shellout("xcrun notarytool submit --apple-id '#{apple_id}' --password '#{password}' --team-id '#{team_id}' '#{pkg_path}'")
        submission_id = status.stdout.match(/^\s*id:\s+(\S+)/)&.captures&.first if status.exitstatus == 0
        break if submission_id
        raise "Failed to submit #{pkg_path} for notarization" if attempt == max_retries - 1
        sleep 2
      end

      max_retries.times do |attempt|
        status = shellout("xcrun notarytool wait --apple-id '#{apple_id}' --password '#{password}' --team-id '#{team_id}' --timeout '#{timeout}' '#{submission_id}'")
        break if status.exitstatus == 0
        raise "Failed waiting for notarization of #{pkg_path}" if attempt == max_retries - 1
        sleep 2
      end

      status = shellout("xcrun stapler staple '#{pkg_path}'")
      raise "Failed to staple #{pkg_path}" unless status.exitstatus == 0

      log.info(self.class.name) { "Notarized and stapled #{pkg_path}" }
    end
  end

  Packager::PKG.prepend PackagerPKGNotarizer

  # Omnibus creates parent directories in the RPM staging tree for every
  # extra_package_file. Those parents are often owned by distribution packages
  # (for example /usr/lib/systemd), and should not be owned by Datadog RPMs.
  # Explicit extra_package_file directories are still kept.
  module PackagerRPMExtraPackageParentFilter
    def build_filepath(path, debug = false)
      filepath = "/" + path.gsub("#{build_dir(debug)}/", "")
      return "" if extra_package_parent_directory?(filepath)

      super
    end

    private

    def extra_package_parent_directory?(filepath)
      extra_package_parent_directories.include?(File.expand_path(filepath))
    end

    def extra_package_parent_directories
      @extra_package_parent_directories ||= begin
        dirs = Set.new
        project_root = File.expand_path(Omnibus::Config.project_root)
        install_root = File.expand_path(project.install_dir)
        Array(project.extra_package_files).each do |path|
          expanded_path = File.expand_path(path)
          next if path_inside?(expanded_path, project_root) || path_inside?(expanded_path, install_root)

          parent = File.dirname(expanded_path)
          while parent != "/"
            dirs.add(parent)
            parent = File.dirname(parent)
          end
        end
        dirs
      end
    end

    def path_inside?(path, root)
      path == root || path.start_with?("#{root}/")
    end
  end

  Packager::RPM.prepend PackagerRPMExtraPackageParentFilter

  # Open the Builder class to allow adding custom DSL methods
  class Builder
    #
    # Runs a command from the root of the datadog-agent repository
    #
    def command_on_repo_root(*args, **kwargs)
      command *args, **kwargs, cwd: File.join(Omnibus::Config.project_root, "..")
    end
    expose :command_on_repo_root
  end
end
