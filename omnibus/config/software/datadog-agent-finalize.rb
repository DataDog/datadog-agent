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
    # Move the deprecation placeholder
    move "#{install_dir}/bin/agent/dd-agent", "/usr/bin/dd-agent"

    # Move checks and configuration files
    mkdir "/etc/datadog-agent"
    move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", "/etc/datadog-agent"
    move "#{install_dir}/etc/datadog-agent/trace-agent.conf", "/etc/datadog-agent"
    move "#{install_dir}/etc/datadog-agent/process-agent.conf", "/etc/datadog-agent"
    move "#{install_dir}/etc/datadog-agent/conf.d", "/etc/datadog-agent"
    move "#{install_dir}/bin/agent/dist/conf.d/*", "/etc/datadog-agent/conf.d"
    move "#{install_dir}/agent/checks.d", "#{install_dir}/checks.d"

    # Move system service files
    mkdir "/etc/init"
    move "#{install_dir}/scripts/datadog-agent.conf", "/etc/init"
    mkdir "/lib/systemd/system"
    move "#{install_dir}/scripts/datadog-agent.service", "/lib/systemd/system"

    # cleanup clutter
    delete "#{install_dir}/etc" if !osx?
    delete "#{install_dir}/sources"
    delete "#{install_dir}/bin/agent/dist/conf.d"
end
