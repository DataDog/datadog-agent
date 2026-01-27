name 'datadog-agent-installer-symlinks'

description "Create symlinks for the datadog-agent installer to track deb/rpm packages as installed"

always_build true

build do
  license :project_license

  block do
    if linux_target? and install_dir == '/opt/datadog-agent'
      version = project.build_version
      mkdir '/opt/datadog-packages/datadog-agent'
      mkdir '/opt/datadog-packages/run/datadog-agent'
      link "/opt/datadog-agent", "/opt/datadog-packages/run/datadog-agent/#{version}"
      link "/opt/datadog-packages/run/datadog-agent/#{version}", "/opt/datadog-packages/datadog-agent/stable"
      link "/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/datadog-agent/experiment"
      project.extra_package_file "/opt/datadog-packages/datadog-agent"
      project.extra_package_file "/opt/datadog-packages/run"

      # TODO: Next PR, enable this and delete the rest.
      # NOTE TO REVIEWER: I will delete these lines before commit. They are only for
      command_on_repo_root "bazelisk run -- //packages/agent/linux:install --destdir='/opt/datadog-packages/pr45475'"
      command_on_repo_root "find /opt/datadog-packages", :live_stream => Omnibus.logger.live_stream(:info)
      command_on_repo_root "ls -lR /opt/datadog-packages", :live_stream => Omnibus.logger.live_stream(:info)
    end
  end
end
