require "./base.rb"

module Omnibus
  class Packager::RPM
    class << self
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
    end

    setup do
      # Create our magic directories
      create_directory("#{staging_dir}/BUILD")
      create_directory("#{staging_dir}/RPMS")
      create_directory("#{staging_dir}/SRPMS")
      create_directory("#{staging_dir}/SOURCES")
      create_directory("#{staging_dir}/SPECS")

      # Create the RPM directory structure for debug builds
      if debug_build?
        create_directory("#{staging_dbg_dir}/BUILD")
        create_directory("#{staging_dbg_dir}/RPMS")
        create_directory("#{staging_dbg_dir}/SRPMS")
        create_directory("#{staging_dbg_dir}/SOURCES")
        create_directory("#{staging_dbg_dir}/SPECS")
      end

      # Copy the full-stack installer into the SOURCE directory, accounting for
      # any excluded files.
      #
      # /opt/hamlet => /tmp/daj29013/BUILD/opt/hamlet
      skip = exclusions + debug_package_paths
      destination = File.join(build_dir, project.install_dir)
      FileSyncer.sync(project.install_dir, destination, exclude: skip)

      if debug_build?
        destination_dbg = File.join(build_dir(true), project.install_dir)
        FileSyncer.sync(project.install_dir, destination_dbg, include: debug_package_paths)
      end

      # Copy over any user-specified extra package files.
      #
      # Files retain their relative paths inside the scratch directory, so
      # we need to grab the dirname of the file, create that directory, and
      # then copy the file into that directory.
      #
      # extra_package_file '/path/to/foo.txt' #=> /tmp/BUILD/path/to/foo.txt
      project.extra_package_files.each do |file|
        parent = File.dirname(file)

        if File.directory?(file)
          destination = File.join(build_dir, file)
          create_directory(destination)
          FileSyncer.sync(file, destination)
        else
          destination = File.join(build_dir, parent)
          create_directory(destination)
          copy_file(file, destination)
        end
      end
    end

  end
end
