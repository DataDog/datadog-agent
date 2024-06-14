name 'package-artifacts'

description "Helper to package an XZ build artifact to deb/rpm/..."

always_build true

build do
  input_dir = ENV['OMNIBUS_PACKAGE_ARTIFACT_DIR']
  # Iterate over all provided intermediate artifacts. There can be the main one
  # which contains all binaries, and an optional debug one with the debuging symbols
  # that have been stripped out during the build
  Dir.glob("*.tar.xz", base: input_dir).each do |input|
    path = File.join(input_dir, input)
    command "tar xf #{path} -C /"
    delete path
  end

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

