name 'init-scripts-iot-agent'

description 'Generate and configure init scripts packaging'

always_build true

build do
  if debian_target?
    # Installs: etc/init/datadog-agent.conf (upstart) and lib/systemd/system/datadog-agent.service.
    command "bazel run --//packages/agent:flavor=iot --//:install_dir='#{install_dir}' -- //packages/agent/iot/debian:install --destdir=/", :live_stream => Omnibus.logger.live_stream(:info)
    project.extra_package_file '/etc/init/datadog-agent.conf'
    project.extra_package_file '/lib/systemd/system/datadog-agent.service'
  elsif redhat_target?
    # Installs: etc/init/datadog-agent.conf (upstart) and usr/lib/systemd/system/datadog-agent.service.
    command "bazel run --//packages/agent:flavor=iot --//:install_dir='#{install_dir}' -- //packages/agent/iot/redhat:install --destdir=/", :live_stream => Omnibus.logger.live_stream(:info)
    project.extra_package_file '/etc/init/datadog-agent.conf'
    project.extra_package_file '/usr/lib/systemd/system/datadog-agent.service'
  elsif suse_target?
    # SUSE does not use upstart; only the systemd unit is installed.
    command "bazel run --//packages/agent:flavor=iot --//:install_dir='#{install_dir}' -- //packages/agent/iot/redhat:systemd_install --destdir=/", :live_stream => Omnibus.logger.live_stream(:info)
    project.extra_package_file '/usr/lib/systemd/system/datadog-agent.service'
  end
end
