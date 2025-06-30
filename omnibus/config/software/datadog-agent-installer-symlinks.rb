name 'datadog-agent-installer-symlinks'

description "Create symlinks for the datadog-agent installer to track deb/rpm packages as installed"

always_build true

build do
  license :project_license

  block do
    if linux_target? and install_dir == '/opt/datadog-agent'
      version = project.build_version
      mkdir '/opt/datadog-packages/datadog-agent'
      link "/opt/datadog-agent", "/opt/datadog-packages/datadog-agent/#{version}"
      link "/opt/datadog-packages/datadog-agent/#{version}", "/opt/datadog-packages/datadog-agent/stable"
      link "/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/datadog-agent/experiment"
      project.extra_package_file "/opt/datadog-packages/datadog-agent"
    end
  end
end

