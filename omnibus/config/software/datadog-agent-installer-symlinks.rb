name 'datadog-agent-installer-symlinks'

description "Create symlinks for the datadog-agent installer to track deb/rpm packages as installed"

always_build true

build do
  license :project_license

  block do
    if not linux_target?
        return
    end
    version = project.build_version
    mkdir '/opt/datadog-packages/datadog-agent'
    link "/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/datadog-agent/#{version}"
    link "/opt/datadog-packages/datadog-agent/experiment", "/opt/datadog-packages/datadog-agent/#{version}"
    link "/opt/datadog-packages/datadog-agent/#{version}", "/opt/datadog-agent"
    project.extra_package_file '/opt/datadog-packages'
  end
end

