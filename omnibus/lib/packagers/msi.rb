require "./lib/ostools.rb"

module Omnibus
  class Packager::MSI

    # Override the default install dir to place the install files in
    # the staging directory. That way we can modify it as we want and it won't
    # impact other packagers, and Omnibus will take care of cleaning it for us after the build.
    def install_dir
      "#{staging_dir}\\install_dir"
    end

    def generate_embedded_archive(version)
      safe_embedded_path = windows_safe_path(install_dir, "embedded#{version}")
      safe_embedded_archive_path = windows_safe_path(install_dir, "embedded#{version}.7z")

      shellout!(
        <<-EOH.split.join(" ").squeeze(" ").strip
          7z a -mx=5 -ms=on #{safe_embedded_archive_path} #{safe_embedded_path}
      EOH
      )
      FileUtils.rm_rf "#{safe_embedded_path}"
    end

    def heat_command
      safe_source_install_dir = windows_safe_path(project.install_dir)
      safe_install_dir = windows_safe_path(install_dir)

      # Create a copy of the install directory
      FileUtils.copy_entry safe_source_install_dir, safe_install_dir

      # Create the embedded zips and delete their folders
      generate_embedded_archive(3)

      if with_python_runtime? "2"
        generate_embedded_archive(2)
      end

      # Return this heat command that points to our safe_install_dir
      <<-EOH.split.join(" ").squeeze(" ").strip
          heat.exe dir "#{safe_install_dir}"
            -nologo -srd -sreg -gg -cg ProjectDir
            -dr PROJECTLOCATION
            -var "var.ProjectSourceDir"
            -out "project-files.wxs"
      EOH
    end

    def fast_msi(val = false)
      # Always false because we don't want CustomActionFastMsi.CA.dll
      false
    end
  end
end
