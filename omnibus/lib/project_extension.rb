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
      if @files_to_sign
        @files_to_sign.each do|file|
          ddwcssign(file)
        end
      end
      super
    end

    def ddwcssign(file)
      log.info(self.class.name) { "Signing #{file}" }
      cmd = Array.new.tap do |arr|
          arr << "dd-wcs"
          arr << "sign"
          arr << "\"#{file}\""
      end.join(" ")
      status = shellout(cmd)
      if status.exitstatus != 0
        log.warn(self.class.name) do
        <<-EOH.strip
          Failed to sign with dd-wcs

          STDOUT
          ------
          #{status.stdout}

          STDERR
          ------
          #{status.stderr}
        EOH
        end
      end
    end

    #
    # Add a signing step for a file.
    #
    def sign_file(file_path)
      @files_to_sign ||= []
      @files_to_sign.append(file_path)
    end
  end

  class Project
    prepend ProjectExtensions
    expose :inspect_binary
    expose :sign_file
  end
end