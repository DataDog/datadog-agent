# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-cluster-agent-finalize"
description "steps required to finalize the build"
default_version "1.0.0"
skip_transitive_dependency_licensing true

build do
    if windows?
        conf_dir_root = "#{Omnibus::Config.source_dir()}/etc/datadog-cluster-agent"
        conf_dir = "#{conf_dir_root}/extra_package_files/EXAMPLECONFSLOCATION"
        mkdir conf_dir
        move "#{install_dir}/etc/datadog-cluster-agent/datadog-cluster.yaml.example", conf_dir_root, :force=>true
    else
        # Move configuration files
        mkdir "/etc/datadog-cluster-agent"
        move "#{install_dir}/etc/datadog-cluster-agent/datadog-cluster.yaml.example", "/etc/datadog-cluster-agent"

        # Move system service filess
        mkdir "/etc/init"
        move "#{install_dir}/scripts/datadog-cluster-agent.conf", "/etc/init"
        mkdir "/lib/systemd/system"
        move "#{install_dir}/scripts/datadog-cluster-agent.service", "/lib/systemd/system"

        mkdir "/etc/datadog-cluster-agent/etc"
        move "#{install_dir}/etc/datadog-cluster-agent/conf.d", "/etc/datadog-cluster-agent/etc/", :force=>true

        # cleanup clutter
        delete "#{install_dir}/etc" if !osx?
        delete "#{install_dir}/bin/datadog-cluster-agent/dist"
    end
end
