# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require 'pathname'

name 'datadog-dogstatsd'

skip_transitive_dependency_licensing true

source path: '..',
       options: {
         exclude: ["**/.cache/**/*", "**/.git/fsmonitor--daemon.ipc"],
       }
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

  if linux_target?
    if debian_target?
      install_target = "//packages/dogstatsd/linux:install_debian"
    else
      install_target = "//packages/dogstatsd/linux:install_redhat"
    end
    # Bazel places the binary, yaml example, init scripts, service file, and
    # creates /etc/datadog-dogstatsd/ and /var/log/datadog/.
    command "bazel run #{omnibazel_flags} -- #{install_target} --destdir=/",
      :live_stream => Omnibus.logger.live_stream(:info)
    mkdir "#{install_dir}/run"
    mkdir "#{install_dir}/scripts"
    project.extra_package_file '/etc/init/datadog-dogstatsd.conf'
    project.extra_package_file '/lib/systemd/system/datadog-dogstatsd.service'
  elsif windows_target?
    # dogstatsd.exe is not installed under install_dir on Windows: it is
    # staged into the agent's WiX harvest directory (see
    # omnibus/config/projects/{dogstatsd,agent-binaries}.rb, 'BinFiles'). The
    # example config's final home (source_dir/etc/datadog-dogstatsd) matches
    # the Bazel target's own layout, so install straight into source_dir and
    # only the binary needs an explicit copy into the WiX-specific path.
    source_dir = Omnibus::Config.source_dir()
    command "bazel run #{omnibazel_flags} -- //packages/dogstatsd/windows:install --destdir=#{source_dir}",
      :live_stream => Omnibus.logger.live_stream(:info)

    mkdir "#{source_dir}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"
    copy "#{source_dir}/bin/dogstatsd.exe", "#{source_dir}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"

    conf_dir_root = "#{source_dir}/etc/datadog-dogstatsd"
    mkdir "#{conf_dir_root}/extra_package_files/EXAMPLECONFSLOCATION"
  else
    # macOS: install directly to install_dir (== /opt/datadog-dogstatsd),
    # where the .pkg will find both the binary and the yaml example.
    command "bazel run #{omnibazel_flags} -- //packages/dogstatsd/macos:install --destdir=/",
      :live_stream => Omnibus.logger.live_stream(:info)
  end
end
