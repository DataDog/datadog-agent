name 'init-scripts-ddot'

description "Generate and configure DDOT init scripts packaging"

always_build true

build do
  output_config_dir = ENV["OUTPUT_CONFIG_DIR"] || ""
  if linux_target?
    etc_dir = "#{output_config_dir}/etc/datadog-agent"
    mkdir "/etc/init"
    if debian_target?
      # sysvinit support for debian only for now
      mkdir "/etc/init.d"

      erb source: "upstart_debian.ddot.conf.erb",
          dest: "/etc/init/datadog-agent-ddot.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.ddot.erb",
          dest: "/etc/init.d/datadog-agent-ddot",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }

      project.extra_package_file '/etc/init.d/datadog-agent-ddot'
    elsif redhat_target? || suse_target?
      # Ship a different upstart job definition on RHEL to accommodate the old
      # version of upstart (0.6.5) that RHEL 6 provides.
      erb source: "upstart_redhat.ddot.conf.erb",
          dest: "/etc/init/datadog-agent-ddot.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
    end
    project.extra_package_file '/etc/init/datadog-agent-ddot.conf'
  end
end
