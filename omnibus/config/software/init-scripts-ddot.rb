name 'init-scripts-ddot'

description "Generate and configure DDOT init scripts packaging"

always_build true

build do
  # This is horrible.
  destdir = "/"
  packager_input = ENV["OMNIBUS_PACKAGE_ARTIFACT_DIR"]
  if linux_target?
    if debian_target?
      command_on_repo_root "bazelisk run --//:install_dir=#{install_dir} -- //packages/ddot/debian:hacky_packager_install --verbose --destdir=#{packager_input}"

    elsif redhat_target? || suse_target?
      command_on_repo_root "bazelisk run --//:install_dir=#{install_dir} -- //packages/ddot/redhat:install --verbose --destdir=#{destdir}"

      project.extra_package_file '/etc/init/datadog-agent-ddot.conf'
    end
  end
end
