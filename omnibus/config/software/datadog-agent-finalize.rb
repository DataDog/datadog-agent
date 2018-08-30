# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-agent-finalize"
description "steps required to finalize the build"
default_version "1.0.0"
skip_transitive_dependency_licensing true

build do
    # TODO too many things done here, should be split
    block do
        # Conf files

        if windows?
            conf_dir_root = "#{Omnibus::Config.source_dir()}/etc/datadog-agent"
            conf_dir = "#{conf_dir_root}/extra_package_files/EXAMPLECONFSLOCATION"
            mkdir conf_dir
            move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", conf_dir_root, :force=>true
            move "#{install_dir}/etc/datadog-agent/root.json", conf_dir_root, :force=>true
            move "#{install_dir}/etc/datadog-agent/conf.d/*", conf_dir, :force=>true
            delete "#{install_dir}/bin/agent/agent.exe"
            # TODO why does this get generated at all
            delete "#{install_dir}/bin/agent/agent.exe~"

            # remove the config files for the subservices; they'll be started
            # based on the config file
            delete "#{conf_dir}/apm.yaml.default"
            # load isn't supported by windows
            delete "#{conf_dir}/load.d"

            # cleanup clutter
            delete "#{install_dir}/etc"
            delete "#{install_dir}/bin/agent/dist/conf.d"
            delete "#{install_dir}/bin/agent/dist/*.conf*"
            delete "#{install_dir}/bin/agent/dist/*.yaml"
        elsif linux?
            # Move system service files
            mkdir "/etc/init"
            move "#{install_dir}/scripts/datadog-agent.conf", "/etc/init"
            move "#{install_dir}/scripts/datadog-agent-trace.conf", "/etc/init"
            move "#{install_dir}/scripts/datadog-agent-process.conf", "/etc/init"
            move "#{install_dir}/scripts/datadog-agent-network.conf", "/etc/init"
            systemd_directory = "/usr/lib/systemd/system"
            if debian?
                # debian recommends using a different directory for systemd unit files
                systemd_directory = "/lib/systemd/system"

                # sysvinit support for debian only for now
                mkdir "/etc/init.d"
                move "#{install_dir}/scripts/datadog-agent", "/etc/init.d"
                move "#{install_dir}/scripts/datadog-agent-trace", "/etc/init.d"
                move "#{install_dir}/scripts/datadog-agent-process", "/etc/init.d"
            end
            mkdir systemd_directory
            move "#{install_dir}/scripts/datadog-agent.service", systemd_directory
            move "#{install_dir}/scripts/datadog-agent-trace.service", systemd_directory
            move "#{install_dir}/scripts/datadog-agent-process.service", systemd_directory
            move "#{install_dir}/scripts/datadog-agent-network.service", systemd_directory

            # Move checks and configuration files
            mkdir "/etc/datadog-agent"
            move "#{install_dir}/bin/agent/dd-agent", "/usr/bin/dd-agent"
            move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", "/etc/datadog-agent"
            move "#{install_dir}/etc/datadog-agent/network-tracer.yaml.example", "/etc/datadog-agent"
            move "#{install_dir}/etc/datadog-agent/conf.d", "/etc/datadog-agent", :force=>true

            # Create empty directories so that they're owned by the package
            # (also requires `extra_package_file` directive in project def)
            mkdir "/etc/datadog-agent/checks.d"
            mkdir "/var/log/datadog"

            # remove unused configs
            delete "/etc/datadog-agent/conf.d/apm.yaml.default"

            # remove windows specific configs
            delete "/etc/datadog-agent/conf.d/winproc.d"

            # cleanup clutter
            delete "#{install_dir}/etc"
        elsif osx?
            # Remove linux specific configs
            delete "#{install_dir}/etc/conf.d/file_handle.d"

            # remove windows specific configs
            delete "#{install_dir}/etc/conf.d/winproc.d"

            # Nothing to move on osx, the confs already live in /opt/datadog-agent/etc/
        end
    end
end

