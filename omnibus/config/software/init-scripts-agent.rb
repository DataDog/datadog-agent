name 'init-scripts-agent'

description "Generate and configure init scripts packaging"

always_build true
skip_transitive_dependency_licensing true

build do
  destdir = ENV["OMNIBUS_BASE_DIR"] || "/"
  output_config_dir = ENV["OUTPUT_CONFIG_DIR"] || ""
  if linux_target?
    etc_dir = "#{output_config_dir}/etc/datadog-agent"
    mkdir "/etc/init"
    if debian_target?
      # building into / is not acceptable. We'll continue to to that for now,
      # but the replacement has to build to a build output tree.
      command_on_repo_root "bazelisk run --//:output_config_dir='#{output_config_dir}' --//:install_dir=#{install_dir} -- //packages/debian/etc:install --verbose --destdir=#{destdir}"

      # sysvinit support for debian only for now
      mkdir "/etc/init.d"

      project.extra_package_file '/etc/init.d/datadog-agent'
      project.extra_package_file '/etc/init.d/datadog-agent-process'
      project.extra_package_file '/etc/init.d/datadog-agent-trace'
      project.extra_package_file '/etc/init.d/datadog-agent-security'
      project.extra_package_file '/etc/init.d/datadog-agent-data-plane'
      project.extra_package_file '/etc/init.d/datadog-agent-action'
    elsif redhat_target? || suse_target?
      command_on_repo_root "bazelisk run --//:output_config_dir='#{output_config_dir}' --//:install_dir=#{install_dir} -- //packages/redhat/etc:install --verbose --destdir=#{destdir}"
    end
    project.extra_package_file '/etc/init/datadog-agent.conf'
    project.extra_package_file '/etc/init/datadog-agent-process.conf'
    project.extra_package_file '/etc/init/datadog-agent-sysprobe.conf'
    project.extra_package_file '/etc/init/datadog-agent-trace.conf'
    project.extra_package_file '/etc/init/datadog-agent-security.conf'
    project.extra_package_file '/etc/init/datadog-agent-data-plane.conf'
    project.extra_package_file '/etc/init/datadog-agent-action.conf'
  end
end
