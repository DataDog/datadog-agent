require "omnibus/packagers/base"

module Omnibus
  class Packager::Base
    #
    # The list of debug paths from the project and softwares.
    #
    # @return [Array<String>]
    #
    def debug_package_paths
      project.library.components.inject(project.debug_package_paths) do |array, component|
        array += component.debug_package_paths
        array
      end
    end

    #
    # Returns whether or not this is a debug build.
    #
    # @return boolean
    #
    def debug_build?
      not debug_package_paths.empty?
    end
  end
end
