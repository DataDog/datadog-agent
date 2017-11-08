# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-agent-finalize"
description "steps required to finalize the build"
default_version "1.0.0"
skip_transitive_dependency_licensing true

build do
    if windows?
        conf_dir_root = "#{Omnibus::Config.source_dir()}/etc/datadog-agent"
        conf_dir = "#{conf_dir_root}/extra_package_files/EXAMPLECONFSLOCATION"
        mkdir conf_dir
        move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", conf_dir_root, :force=>true
        move "#{install_dir}/etc/datadog-agent/trace-agent.conf", conf_dir_root, :force=>true
        #move "#{install_dir}/etc/datadog-agent/process-agent.conf", conf_dir
        move "#{install_dir}/etc/datadog-agent/conf.d/*", conf_dir, :force=>true
        #move "#{install_dir}/agent/checks.d", conf_dir
        delete "#{install_dir}/bin/agent/agent.exe"
        # TODO why does this get generated at all
        delete "#{install_dir}/bin/agent/agent.exe~"
    else
        # Move checks and configuration files
        mkdir "/etc/datadog-agent"
        move "#{install_dir}/bin/agent/dd-agent", "/usr/bin/dd-agent"
        move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", "/etc/datadog-agent"
        move "#{install_dir}/etc/datadog-agent/trace-agent.conf", "/etc/datadog-agent/trace-agent.conf.example"
        move "#{install_dir}/etc/datadog-agent/process-agent.conf", "/etc/datadog-agent/process-agent.conf.example"
        move "#{install_dir}/etc/datadog-agent/conf.d", "/etc/datadog-agent", :force=>true
        move "#{install_dir}/bin/agent/dist/conf.d/*.yaml*", "/etc/datadog-agent/conf.d/"
        Dir.glob("#{install_dir}/bin/agent/dist/conf.d/**/*.d").each do |check_dir|
            dir_name = File.basename check_dir
            mkdir "/etc/datadog-agent/conf.d/#{dir_name}" unless File.exists? "/etc/datadog-agent/conf.d/#{dir_name}"
            move "#{install_dir}/bin/agent/dist/conf.d/#{dir_name}/*.yaml*", "/etc/datadog-agent/conf.d/#{dir_name}/", :force=>true
        end
        move "#{install_dir}/agent/checks.d", "#{install_dir}/checks.d"

        # Move system service files
        mkdir "/etc/init"
        move "#{install_dir}/scripts/datadog-agent.conf", "/etc/init"
        mkdir "/lib/systemd/system"
        move "#{install_dir}/scripts/datadog-agent.service", "/lib/systemd/system"

        # cleanup clutter
        delete "#{install_dir}/etc" if !osx?
        delete "#{install_dir}/bin/agent/dist/conf.d"
        delete "#{install_dir}/bin/agent/dist/*.conf"
        delete "#{install_dir}/bin/agent/dist/*.yaml"
    end
end
