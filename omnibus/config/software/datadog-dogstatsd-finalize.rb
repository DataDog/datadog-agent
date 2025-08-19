# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-dogstatsd-finalize"
description "steps required to finalize the build"
default_version "1.0.0"

skip_transitive_dependency_licensing true

build do
    license :project_license

    if windows_target?
        conf_dir_root = "#{Omnibus::Config.source_dir()}/etc/datadog-dogstatsd"
        conf_dir = "#{conf_dir_root}/extra_package_files/EXAMPLECONFSLOCATION"
        mkdir conf_dir
        move "#{install_dir}/etc/datadog-dogstatsd/dogstatsd.yaml.example", conf_dir_root, :force=>true
    else
        # Move configuration files
        mkdir "/etc/datadog-dogstatsd"
        move "#{install_dir}/etc/datadog-dogstatsd/dogstatsd.yaml.example", "/etc/datadog-dogstatsd"

        # Move system service files
        mkdir "/etc/init"
        move "#{install_dir}/scripts/datadog-dogstatsd.conf", "/etc/init"
        mkdir "/lib/systemd/system"
        move "#{install_dir}/scripts/datadog-dogstatsd.service", "/lib/systemd/system"
        mkdir "/var/log/datadog"

        # cleanup clutter
        delete "#{install_dir}/etc" if !osx_target?
        delete "#{install_dir}/bin/dogstatsd/dist"
    end
end
