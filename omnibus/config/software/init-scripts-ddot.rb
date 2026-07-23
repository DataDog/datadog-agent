name 'init-scripts-ddot'

description "Generate and configure DDOT init scripts packaging"

always_build true

build do
  # This is horrible.
  destdir = "/"
  if linux_target?
    if debian_target?
      command "bazel run #{omnibazel_flags} -- //packages/ddot/debian:install --verbose --destdir=#{destdir}",
        :live_stream => Omnibus.logger.live_stream(:info)

      project.extra_package_file '/etc/init.d/datadog-agent-ddot'
      project.extra_package_file '/etc/init/datadog-agent-ddot.conf'
    elsif redhat_target? || suse_target?
      command "bazel run #{omnibazel_flags} -- //packages/ddot/redhat:install --verbose --destdir=#{destdir}",
        :live_stream => Omnibus.logger.live_stream(:info)

      project.extra_package_file '/etc/init/datadog-agent-ddot.conf'
    end
  end
end
