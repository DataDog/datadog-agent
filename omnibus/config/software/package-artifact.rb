name 'package-artifacts'

description "Helper to package an XZ build artifact to deb/rpm/..."

build do
  command "tar xf #{ENV['OMNIBUS_PACKAGE_ARTIFACT']} -C /"
  delete "#{ENV['OMNIBUS_PACKAGE_ARTIFACT']}"

  if project.name == "installer"
    # This file depends on the type of package and must therefor be generated during
    # packaging, not building.
    uninstall_command="sudo yum remove datadog-installer"
    if debian_target?
        uninstall_command="sudo apt-get remove datadog-installer"
    end
    # Omnibus hardcodes the template rendering to be in config/templates/<software-name>
    # so we need to move the input to its expected location
    FileUtils.mkdir_p "#{Omnibus::Config.project_root()}/config/templates/package-artifacts"
    FileUtils.move "#{Omnibus::Config.project_root()}/config/templates/installer/README.md.erb", "#{Omnibus::Config.project_root()}/config/templates/package-artifacts/README.md.erb"
    erb source: "README.md.erb",
       dest: "#{install_dir}/README.md",
       mode: 0644,
       vars: { uninstall_command: uninstall_command}
  end
end

