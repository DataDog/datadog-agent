module Omnibus
  class Packager::MSI
    class << self
      def build(&block)
        if block
          @build = -> {
            if signing_identity
              puts "starting signing"
              if additional_sign_files
                additional_sign_files.each do |signfile|
                  puts "signing #{signfile}"
                  sign_package(signfile)
                end
              end
            end
          } >> block
        else
          @build
        end
      end
    end

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

    def extra_package_dir(val = NULL)
      if null?(val)
        @extra_package_dir || nil
      else
        unless val.is_a?(String)
          raise InvalidValue.new(:extra_package_dir, "be a String")
        end

        @extra_package_dir = val
      end
    end
    expose :extra_package_dir

    #
    # Returns a list of dir refs from extra_package_dir
    #
    def extra_package_dir_ref
      dir_refs = []
      unless extra_package_dir.nil?
        if File.directory?(extra_package_dir)
          # Let's collect the DirectoryRefs
          Dir.foreach(extra_package_dir) do |item|
            next if item == '.' or item == '..'
            dir_refs.push(item)
          end
        end
      end
      dir_refs
    end

    #
    # Get the shell command to run heat in order to create a
    # a WIX manifest of project files to be packaged into the MSI
    #
    # @return [String]
    #
    def heat_command
      if fast_msi
        <<-EOH.split.join(" ").squeeze(" ").strip
          heat.exe file "#{project.name}.zip"
          -cg ProjectDir
          -dr INSTALLLOCATION
          -nologo -sfrag -srd -sreg -gg
          -out "project-files.wxs"
        EOH
      else
        heat = <<-EOH.split.join(" ").squeeze(" ").strip
          heat.exe dir "#{windows_safe_path(project.install_dir)}"
            -nologo -srd -sreg -gg -cg ProjectDir
            -dr PROJECTLOCATION
            -var "var.ProjectSourceDir"
            -out "project-files.wxs"
        EOH

        # If there are extra package files let's Harvest them hard
        extra_package_dir_ref.each do |dirref|
          heat += <<-EOH.split.join(' ').squeeze(' ').strip
            && heat.exe dir
              "#{windows_safe_path("#{extra_package_dir}\\#{dirref}")}"
              -nologo -srd -gg -cg Extra#{dirref}
              -dr #{dirref}
              -var "var.Extra#{dirref}"
              -out "extra-#{dirref}.wxs"
          EOH
        end

        heat
      end
    end

    #
    # Get the shell command to compile the project WIX files
    #
    # @return [String]
    #
    def candle_command(is_bundle: false)
      candle_vars = ''
      wxs_list = ''
      extra_package_dir_ref.each do |dirref|
        candle_vars += "-dExtra#{dirref}=\""\
            "#{windows_safe_path("#{extra_package_dir}\\#{dirref}")}"\
            "\" "
        wxs_list += "extra-#{dirref}.wxs "
      end

      if is_bundle
        <<-EOH.split.join(" ").squeeze(" ").strip
        candle.exe
          -nologo
          #{wix_candle_flags}
          -ext WixBalExtension
          #{wix_extension_switches(wix_candle_extensions)}
          -dOmnibusCacheDir="#{windows_safe_path(File.expand_path(Config.cache_dir))}"
          #{candle_vars}
          "#{windows_safe_path(staging_dir, "bundle.wxs")}"
        EOH
      else
        <<-EOH.split.join(" ").squeeze(" ").strip
          candle.exe
            -nologo
            #{wix_candle_flags}
            #{wix_extension_switches(wix_candle_extensions)}
            -dProjectSourceDir="#{windows_safe_path(project.install_dir)}"
            #{candle_vars}
            "project-files.wxs"
            #{wxs_list}
            "#{windows_safe_path(staging_dir, "source.wxs")}"
        EOH
      end
    end

    #
    # Get the shell command to link the project WIX object files
    #
    # @return [String]
    #
    def light_command(out_file, is_bundle: false)
      if is_bundle
        <<-EOH.split.join(" ").squeeze(" ").strip
        light.exe
          -nologo
          #{wix_light_delay_validation}
          -ext WixUIExtension
          -ext WixBalExtension
          #{wix_extension_switches(wix_light_extensions)}
          -cultures:#{localization}
          -loc "#{windows_safe_path(staging_dir, "localization-#{localization}.wxl")}"
          bundle.wixobj
          -out "#{out_file}"
        EOH
      else
        wixobj_list = "project-files.wixobj source.wixobj"
        extra_package_dir_ref.each do |dirref|
          wixobj_list += " extra-#{dirref}.wixobj"
        end
        <<-EOH.split.join(" ").squeeze(" ").strip
          light.exe
            -nologo
            #{wix_light_delay_validation}
            -ext WixUIExtension
            #{wix_extension_switches(wix_light_extensions)}
            -cultures:#{localization}
            -loc "#{windows_safe_path(staging_dir, "localization-#{localization}.wxl")}"
            #{wixobj_list}
            -out "#{out_file}"
        EOH
      end
    end
  end
end
