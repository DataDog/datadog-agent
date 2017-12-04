name 'datadog-agent-mac-app'

description "Generate mac app manifest and assets"

dependency "datadog-agent"

source path: "#{project.files_path}/#{name}"

# This needs to be done in a separate software because we need to know the Agent Version to build the app
# manifest, and `project.build_version` is populated only once the software that the project
# takes its version from (i.e. `datadog-agent`) has finished building
build do
    app_temp_dir = "#{install_dir}/Datadog Agent.app/Contents"
    mkdir "#{app_temp_dir}/Resources"
    copy "Agent.icns", "#{app_temp_dir}/Resources/"

    block do # defer in a block to allow getting the project's build version
      erb source: "Info.plist.erb",
          dest: "#{app_temp_dir}/Info.plist",
          mode: 0755,
          vars: { version: project.build_version, year: Time.now.year, executable: "gui" }
    end
end
