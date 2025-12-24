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
      command_on_repo_root "cp bazel-bin/packages/debian/etc/etc/init/datadog-agent.conf /etc/init"

      # sysvinit support for debian only for now
      mkdir "/etc/init.d"

      erb source: "upstart_debian.process.conf.erb",
          dest: "/etc/init/datadog-agent-process.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.sysprobe.conf.erb",
          dest: "/etc/init/datadog-agent-sysprobe.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.trace.conf.erb",
          dest: "/etc/init/datadog-agent-trace.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.security.conf.erb",
          dest: "/etc/init/datadog-agent-security.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.data-plane.conf.erb",
          dest: "/etc/init/datadog-agent-data-plane.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.erb",
          dest: "/etc/init.d/datadog-agent",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.process.erb",
          dest: "/etc/init.d/datadog-agent-process",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.trace.erb",
          dest: "/etc/init.d/datadog-agent-trace",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.security.erb",
          dest: "/etc/init.d/datadog-agent-security",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.data-plane.erb",
          dest: "/etc/init.d/datadog-agent-data-plane",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }

      project.extra_package_file '/etc/init.d/datadog-agent'
      project.extra_package_file '/etc/init.d/datadog-agent-process'
      project.extra_package_file '/etc/init.d/datadog-agent-trace'
      project.extra_package_file '/etc/init.d/datadog-agent-security'
      project.extra_package_file '/etc/init.d/datadog-agent-data-plane'
    elsif redhat_target? || suse_target?
      # Ship a different upstart job definition on RHEL to accommodate the old
      # version of upstart (0.6.5) that RHEL 6 provides.
      erb source: "upstart_redhat.conf.erb",
          dest: "/etc/init/datadog-agent.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.process.conf.erb",
          dest: "/etc/init/datadog-agent-process.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.sysprobe.conf.erb",
          dest: "/etc/init/datadog-agent-sysprobe.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.trace.conf.erb",
          dest: "/etc/init/datadog-agent-trace.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.security.conf.erb",
          dest: "/etc/init/datadog-agent-security.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.data-plane.conf.erb",
          dest: "/etc/init/datadog-agent-data-plane.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
    end
    project.extra_package_file '/etc/init/datadog-agent.conf'
    project.extra_package_file '/etc/init/datadog-agent-process.conf'
    project.extra_package_file '/etc/init/datadog-agent-sysprobe.conf'
    project.extra_package_file '/etc/init/datadog-agent-trace.conf'
    project.extra_package_file '/etc/init/datadog-agent-security.conf'
    project.extra_package_file '/etc/init/datadog-agent-data-plane.conf'
  end
end
