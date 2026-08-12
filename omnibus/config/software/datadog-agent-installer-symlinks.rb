name 'datadog-agent-installer-symlinks'

description "Create symlinks for the datadog-agent installer to track deb/rpm packages as installed"

always_build true
skip_transitive_dependency_licensing true

build do
  license :project_license

  block do
    if linux_target? and install_dir == '/opt/datadog-agent'
      destdir = ENV["OMNIBUS_BASE_DIR"] || "/"
      command "bazel run #{omnibazel_flags} -- //packages/agent/linux:install --destdir='#{destdir}'",
        :live_stream => Omnibus.logger.live_stream(:info)
      project.extra_package_file "/opt/datadog-packages/datadog-agent"
      project.extra_package_file "/opt/datadog-packages/run"
      # private action runner: pkg/privateactionrunner/autoconnections/conf
      # It does not really belong here, but this script should be renamed to
      # agent-etc and include all the etc things.  But really it should go away entirely
      # by the end of Q1, so I'm not going to create a new .rb file just to have another install
      # target and this extra_package_file
      project.extra_package_file "/etc/datadog-agent"
    end
  end
end
