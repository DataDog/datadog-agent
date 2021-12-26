module Omnibus
  class Software
    def sources_dir
      @project.sources_dir
    end

    expose :sources_dir

    def python_2_embedded
      @project.python_2_embedded
    end

    expose :python_2_embedded

    def python_3_embedded
      @project.python_3_embedded
    end

    expose :python_3_embedded

    #
    # Set or retrieve the source for the software.
    #
    # @raise [InvalidValue]
    #   if the parameter is not a Hash
    # @raise [InvalidValue]
    #   if the hash includes extraneous keys
    # @raise [InvalidValue]
    #   if the hash declares keys that cannot work together
    #   (like +:git+ and +:path+)
    #
    # @example
    #   source url: 'http://ftp.gnu.org/gnu/autoconf/autoconf-2.68.tar.gz',
    #          md5: 'c3b5247592ce694f7097873aa07d66fe'
    #
    # @param [Hash<Symbol, String>] val
    #   a single key/pair that defines the kind of source and a path specifier
    #
    # @option val [String] :git (nil)
    #   a git URL
    # @option val [String] :github (nil)
    #   a github ORG/REPO pair (e.g. chef/chef) that will be transformed to https://github.com/ORG/REPO.git
    # @option val [String] :url (nil)
    #   general URL
    # @option val [String] :path (nil)
    #   a fully-qualified local file system path
    # @option val [String] :md5 (nil)
    #   the MD5 checksum of the downloaded artifact
    # @option val [String] :sha1 (nil)
    #   the SHA1 checksum of the downloaded artifact
    # @option val [String] :sha256 (nil)
    #   the SHA256 checksum of the downloaded artifact
    # @option val [String] :sha512 (nil)
    #   the SHA512 checksum of the downloaded artifact
    #
    # Only used in net_fetcher:
    #
    # @option val [String] :cookie (nil)
    #   a cookie to set
    # @option val [String] :warning (nil)
    #   a warning message to print when downloading
    # @option val [Symbol] :extract (nil)
    #   either :tar, :lax_tar :seven_zip
    # @option val [String] :target_filename (nil)
    #   when the source is a single (non-extractable) file, the file will be present under this name
    #   in the project_dir.
    #   Defaults to "#{software.name}-#{URLBasename}"
    #
    # Only used in path_fetcher:
    #
    # @option val [Hash] :options (nil)
    #   flags/options that are passed through to file_syncer in path_fetcher
    #
    # Only used in git_fetcher:
    #
    # @option val [Boolean] :submodules (false)
    #   clone git submodules
    # @option val [Boolean] :always_fetch_tags (false)
    #   always fetch tags from the remote, useful if the version of the project is determined from this software
    #
    # If multiple checksum types are provided, only the strongest will be used.
    #
    # @return [Hash]
    #
    def source(val = NULL)
      unless null?(val)
        unless val.is_a?(Hash)
          raise InvalidValue.new(:source,
                                 "be a kind of `Hash', but was `#{val.class.inspect}'")
        end

        val = canonicalize_source(val)

        extra_keys = val.keys - [
          :git, :path, :url, # fetcher types
          :md5, :sha1, :sha256, :sha512, # hash type - common to all fetchers
          :cookie, :warning, :unsafe, :extract, :target_filename, # used by net_fetcher
          :options, # used by path_fetcher
          :submodules, :always_fetch_tags, # used by git_fetcher
        ]
        unless extra_keys.empty?
          raise InvalidValue.new(:source,
                                 "only include valid keys. Invalid keys: #{extra_keys.inspect}")
        end

        duplicate_keys = val.keys & [:git, :path, :url]
        unless duplicate_keys.size < 2
          raise InvalidValue.new(:source,
                                 "not include duplicate keys. Duplicate keys: #{duplicate_keys.inspect}")
        end

        if ship_source
          val[:ship_source] = true
        end

        @source ||= {}
        @source.merge!(val)
      end

      override = canonicalize_source(overrides[:source])
      apply_overrides(:source, override)
    end

    expose :source

    #
    # A proxy method to the underlying Ohai system.
    #
    # @example
    #   ohai['platform_family']
    #
    # @return [Ohai]
    #
    def ohai
      Ohai
    end

    expose :ohai

    #
    # Downloads a software license to ship with the final build.
    #
    # Licenses will be copied into {install_dir}/sources/{software_name}
    #
    # @param [String] name_or_url
    #   the name of the license to ship or a URL pointing to the license file.
    #
    #   Available License Names : LGPLv2, LGPLv3, PSFL, Apache, Apachev2,
    #   GPLv2, GPLv3, ZPL
    #
    # @example
    #   ship_license 'GPLv3'
    #
    # @example
    #    ship_license 'http://www.r-project.org/Licenses/GPL-3'
    #
    def ship_license(name_or_url)
      @ship_license
    end

    expose :ship_license

    #
    # Tells if the sources should be shipped alongside the built
    # software.
    #
    # If set to true, the sources will be put in {sources_dir}/{software_name}
    # by the fetcher, and will be copied to {install_dir}/sources/{software_name}
    # by the Project which created this Software.
    #
    # @example
    #   ship_source true
    #
    # @param [Boolean] val
    #   the new value of ship_source.
    #
    # @return [Boolean]
    #
    def ship_source(val = NULL)
      if null?(val)
        @ship_source || false
      else
        @ship_source = val
      end
    end

    expose :ship_source

    #
    # [MacOS only]
    # Return the code signing identity. Can be used in software definitions
    # to sign components of the package.
    # Inherited from the parent project.
    #
    # @return [String]
    #   the code-signing identity
    #
    def code_signing_identity
      @project.code_signing_identity
    end

    expose :code_signing_identity

    #
    # [MacOS only]
    # Return the location of the Entitlements file. Can be used
    # in software definitions to specify entitlements when signing files.
    # Inherited from the parent project.
    #
    # @return [String]
    #   the Entitlements file location
    #
    def entitlements_file
      @project.entitlements_file
    end

    expose :entitlements_file
  end
end
