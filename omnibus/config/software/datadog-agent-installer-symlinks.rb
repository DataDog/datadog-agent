name 'datadog-agent-installer-symlinks'

description "Create symlinks for the datadog-agent installer to track deb/rpm packages as installed"

always_build true
skip_transitive_dependency_licensing true

build do
  license :project_license

  block do
    if linux_target? and install_dir == '/opt/datadog-agent'
      command_on_repo_root "bazelisk run -- //packages/agent/linux:install --destdir='/'"
      project.extra_package_file "/opt/datadog-packages/datadog-agent"
      project.extra_package_file "/opt/datadog-packages/run"
    end
  end
end
