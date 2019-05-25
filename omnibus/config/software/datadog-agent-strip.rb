# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-agent-strip"
description "strip symbols from the build"
default_version "1.0.0"
skip_transitive_dependency_licensing true

build do
    block do
        # only stripping linux for now
        if linux?
            # Strip symbols
            strip_symbols(install_dir, "#{install_dir}/symbols")
            delete "#{install_dir}/symbols"
        end
    end
end
