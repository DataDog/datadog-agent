require "./base.rb"

module Omnibus
  class Packager::DEB
    class << self
      def build(&block)
        if block
          @build = block >> -> {
            # Now the debug build
            if debug_build?
              # TODO
            end
          }
        else
          @build
        end
      end
    end

    #
    # Set or return the the gpg key name to use while signing.
    # If this value is provided, Omnibus will attempt to sign the DEB.
    #
    # @example
    #   gpg_key_name 'My key <my.address@here.com>'
    #
    # @param [String] val
    #   the name of the GPG key to use during RPM signing
    #
    # @return [String]
    #   the RPM signing GPG key name
    #
    def gpg_key_name(val = NULL)
      if null?(val)
        @gpg_key_name
      else
        @gpg_key_name = val
      end
    end
    expose :gpg_key_name
  end
end
