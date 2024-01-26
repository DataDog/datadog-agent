# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-agentless-scanner-finalize"
description "steps required to finalize the build"
default_version "1.0.0"

skip_transitive_dependency_licensing true

build do
    license :project_license

    if !windows_target?
        # Move system service files
        mkdir "/lib/systemd/system"
        move "#{install_dir}/scripts/datadog-agentless-scanner.service", "/lib/systemd/system"
        mkdir "/var/log/datadog"

        # cleanup clutter
        delete "#{install_dir}/etc" if !osx_target?
    end
end
