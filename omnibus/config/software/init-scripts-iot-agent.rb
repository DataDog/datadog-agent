name 'init-scripts-iot-agent'

description 'Generate and configure init scripts packaging'

always_build true

build do
  # Upstart
  etc_dir = "/etc/datadog-agent"
  if debian_target?
    mkdir "/etc/init"
    erb source: 'upstart_debian.conf.erb',
        dest: '/etc/init/datadog-agent.conf',
        mode: 0o644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    project.extra_package_file '/etc/init/datadog-agent.conf'
  elsif redhat_target?
    mkdir "/etc/init"
    # Ship a different upstart job definition on RHEL to accommodate the old
    # version of upstart (0.6.5) that RHEL 6 provides.
    erb source: 'upstart_redhat.conf.erb',
        dest: '/etc/init/datadog-agent.conf',
        mode: 0o644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    project.extra_package_file '/etc/init/datadog-agent.conf'
  end

  # Systemd
  if debian_target?
    mkdir '/lib/systemd/system/'
    erb source: 'systemd.service.erb',
        dest: '/lib/systemd/system/datadog-agent.service',
        mode: 0o644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    project.extra_package_file '/lib/systemd/system/datadog-agent.service'
  elsif redhat_target?
    mkdir '/usr/lib/systemd/system/'
    erb source: 'systemd.service.erb',
        dest: '/usr/lib/systemd/system/datadog-agent.service',
        mode: 0o644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    project.extra_package_file '/usr/lib/systemd/system/datadog-agent.service'
  end
end
