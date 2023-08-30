require './lib/symbols_inspector'

module Omnibus
  module ProjectExtensions
    def install_sources
      # Unfortunately the stripper runs before the `package_me` method is called
      # so we have to override `install_sources` which runs before the stripper
      # to inspect non-stripped binaries
      @inspectors.each do|i|
        i.inspect()
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
  end

  class Project
    prepend ProjectExtensions
    expose :inspect_binary
  end
end