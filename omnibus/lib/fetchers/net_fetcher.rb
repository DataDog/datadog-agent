module Omnibus
  class NetFetcher
    #
    # Download the given file using Ruby's +OpenURI+ implementation. This method
    # may emit warnings as defined in software definitions using the +:warning+
    # key.
    #
    # @return [void]
    #
    def download
      log.warn(log_key) { source[:warning] } if source.key?(:warning)

      options = {}

      if source[:unsafe]
        log.warn(log_key) { "Permitting unsafe redirects!" }
        options[:allow_unsafe_redirects] = true
      end

      # Set the cookie if one was given
      options["Cookie"] = source[:cookie] if source[:cookie]
      options["Authorization"] = source[:authorization] if source[:authorization]

      # The s3 bucket isn't public, force downloading using the sdk
      if Config.use_s3_caching && Config.s3_authenticated_download
        get_from_s3
      else
        log.info(log_key) { "Fetching file from `#{download_url}'" }
        download_file!(download_url, downloaded_file, options)
      end
    end

    #
    # Download the file directly from s3 using get_object
    #
    def get_from_s3
      log.info(log_key) { "Fetching file from S3 object `#{S3Cache.key_for(self)}' in bucket `#{Config.s3_bucket}'" }
      begin
        S3Cache.get_object(downloaded_file, self)
      rescue Aws::S3::Errors::NoSuchKey => e
        log.error(log_key) {
          "Download failed - #{e.class}!"
        }
      end
    end

    #
    # The target filename to copy the downloaded file as.
    # Defaults to {#downloaded_file} unless overriden on the source.
    #
    # @return [String]
    #
    def target_filename
      source[:target_filename] || downloaded_file
    end

    #
    # Tells if the sources should be shipped
    #
    # @return [Boolean]
    #
    def ship_source?
      source[:ship_source]
    end

    #
    # Extract the downloaded file, using the magical logic based off of the
    # ending file extension. In the rare event the file cannot be extracted, it
    # is copied over as a raw file.
    #
    #
    # Extract the downloaded file, using the magical logic based off of the
    # ending file extension. In the rare event the file cannot be extracted, it
    # is copied over as a raw file.
    #
    def deploy
      if downloaded_file.end_with?(*ALL_EXTENSIONS)
        log.info(log_key) { "Extracting `#{safe_downloaded_file}' to `#{safe_project_dir}'" }
        extract
      else
        log.info(log_key) { "`#{safe_downloaded_file}' is not an archive - copying to `#{safe_project_dir}'" }

        if File.directory?(downloaded_file)
          # If the file itself was a directory, copy the whole thing over. This
          # seems unlikely, because I do not think it is a possible to download
          # a folder, but better safe than sorry.
          FileUtils.cp_r("#{downloaded_file}/.", project_dir)
        else
          # In the more likely case that we got a "regular" file, we want that
          # file to live **inside** the project directory. project_dir should already
          # exist due to create_required_directories
          log.info(log_key) { "`#{safe_downloaded_file}' is a regular file - naming copy `#{target_filename}'" }
          FileUtils.cp(downloaded_file, File.join(project_dir, target_filename))
        end
      end
      if ship_source?
        FileUtils.mkdir_p("#{sources_dir}/#{name}")
        log.info(log_key) { "Moving the sources #{sources_dir}/#{name}/#{downloaded_file.split("/")[-1]}" }
        if File.directory?(downloaded_file)
          FileUtils.cp_r("#{downloaded_file}/.", "#{sources_dir}/#{name}")
        else
          FileUtils.cp(downloaded_file, "#{sources_dir}/#{name}")
        end
      end
    end

    #
    # Extracts the downloaded archive file into project_dir.
    #
    # On windows, this is a fuster cluck and we allow users to specify the
    # preferred extractor to be used. The default is to use tar. User overrides
    # can be set in source[:extract] as:
    #   :tar - use tar.exe and fail on errors (default strategy).
    #   :seven_zip - use 7zip for all tar/compressed tar files on windows.
    #   :lax_tar - use tar.exe on windows but ignore errors.
    #
    # Both 7z and bsdtar have issues on windows.
    #
    # 7z cannot extract and untar at the same time. You need to extract to a
    # temporary location and then extract again into project_dir.
    #
    # 7z also doesn't handle symlinks well. A symlink to a non-existent
    # location simply results in a text file with the target path written in
    # it. It does this without throwing any errors.
    #
    # bsdtar will exit(1) if it is encounters symlinks on windows. So we can't
    # use shellout! directly.
    #
    # bsdtar will also exit(1) and fail to overwrite files at the destination
    # during extraction if a file already exists at the destination and is
    # marked read-only. This used to be a problem when we weren't properly
    # cleaning an existing project_dir. It should be less of a problem now...
    # but who knows.
    #
    def extract
      # Only used by tar
      compression_switch = ""
      compression_switch = "z"        if downloaded_file.end_with?("gz")
      compression_switch = "--lzma -" if downloaded_file.end_with?("lzma")
      compression_switch = "j"        if downloaded_file.end_with?("bz2")
      compression_switch = "J"        if downloaded_file.end_with?("xz")

      if Ohai["platform"] == "windows"
        if downloaded_file.end_with?(*TAR_EXTENSIONS) && source[:extract] != :seven_zip
          returns = [0]
          returns << 1 if source[:extract] == :lax_tar

          shellout!("tar #{compression_switch}xf #{safe_downloaded_file} -C#{safe_project_dir} --force-local || \
                     tar #{compression_switch}xf #{safe_downloaded_file} -C#{safe_project_dir} ", returns: returns)
        elsif downloaded_file.end_with?(*COMPRESSED_TAR_EXTENSIONS)
          Dir.mktmpdir do |temp_dir|
            log.debug(log_key) { "Temporarily extracting `#{safe_downloaded_file}' to `#{temp_dir}'" }

            shellout!("7z.exe x #{safe_downloaded_file} -o#{windows_safe_path(temp_dir)} -r -y")

            fname = File.basename(downloaded_file, File.extname(downloaded_file))
            fname << ".tar" if downloaded_file.end_with?("tgz", "txz")
            next_file = windows_safe_path(File.join(temp_dir, fname))
            next_file = Dir.glob(File.join(temp_dir, '**', '*.tar'))[0] unless File.file?(next_file)

            log.debug(log_key) { "Temporarily extracting `#{next_file}' to `#{safe_project_dir}'" }
            shellout!("7z.exe x #{next_file} -o#{safe_project_dir} -r -y")
          end
        else
          shellout!("7z.exe x #{safe_downloaded_file} -o#{safe_project_dir} -r -y")
        end
      elsif downloaded_file.end_with?(".7z")
        shellout!("7z x #{safe_downloaded_file} -o#{safe_project_dir} -r -y")
      elsif downloaded_file.end_with?(".zip")
        shellout!("unzip #{safe_downloaded_file} -d #{safe_project_dir}")
      else
        shellout!("#{tar} #{compression_switch}xfo #{safe_downloaded_file} -C#{safe_project_dir}")
      end
    end
  end
end
