require "./lib/ostools.rb"

module Omnibus
  class Packager::MSI

    def safe_install_dir
      "#{windows_safe_path(staging_dir)}\\install_dir"
    end

    def heat_command

      # Create a copy of the install directory
      FileUtils.copy_entry "#{windows_safe_path(project.install_dir)}", safe_install_dir

      # Create the embedded zips and delete their folders
      shellout!(
        <<-EOH.split.join(" ").squeeze(" ").strip
          7z a -mx=5 -ms=on #{windows_safe_path(safe_install_dir)}\\embedded3.7z #{windows_safe_path(safe_install_dir)}\\embedded3
        EOH
      )
      FileUtils.rmdir "#{safe_install_dir}\\embedded3"

      if with_python_runtime? "2"
        shellout!(
          <<-EOH.split.join(" ").squeeze(" ").strip
            7z a -mx=5 -ms=on #{windows_safe_path(safe_install_dir)}\\embedded2.7z #{windows_safe_path(safe_install_dir)}\\embedded2
          EOH
        )
        FileUtils.rmdir "#{safe_install_dir}\\embedded2"
      end

      # Return this heat command that points to our safe_install_dir
      <<-EOH.split.join(" ").squeeze(" ").strip
          heat.exe dir "#{windows_safe_path(safe_install_dir)}"
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
