name 'init-scripts-ddot'

description "Generate and configure DDOT init scripts packaging"

always_build true

build do
  # This is horrible.
  destdir = "/"
  output_config_dir = ENV["OUTPUT_CONFIG_DIR"] || ""
  if linux_target?
    if debian_target?
      command_on_repo_root "bazelisk run --//:output_config_dir='#{output_config_dir}' --//:install_dir=#{install_dir} -- //packages/ddot/debian:install --verbose --destdir=#{destdir}"

      project.extra_package_file '/etc/init.d/datadog-agent-ddot'
      project.extra_package_file '/etc/init/datadog-agent-ddot.conf'
    elsif redhat_target? || suse_target?
      command_on_repo_root "bazelisk run --//:output_config_dir='#{output_config_dir}' --//:install_dir=#{install_dir} -- //packages/ddot/redhat:install --verbose --destdir=#{destdir}"

      project.extra_package_file '/etc/init/datadog-agent-ddot.conf'
    end
  end
end
