name 'datadog-agent-mac-app'

description "Generate mac app manifest and assets"

dependency "datadog-agent"

always_build true

# This needs to be done in a separate software because we need to know the Agent Version to build the app
# manifest, and `project.build_version` is populated only once the software that the project
# takes its version from (i.e. `datadog-agent`) has finished building
build do
    license :project_license

    app_temp_dir = "#{install_dir}/Datadog Agent.app/Contents"
    mkdir "#{app_temp_dir}/Frameworks"

    # Add swift runtime libs in Frameworks (same thing a full xcode build would do for a GUI app).
    # They are needed for the gui to run on MacOS 10.14.3 and lower
    command "$(xcrun --find swift-stdlib-tool) --copy --scan-executable \"#{app_temp_dir}/MacOS/gui\" --scan-folder \"#{app_temp_dir}/Frameworks\" --platform macosx --destination \"#{app_temp_dir}/Frameworks\" --strip-bitcode"
end
