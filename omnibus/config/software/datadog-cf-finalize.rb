# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-cf-finalize"
description "steps required to finalize the CF build"
default_version "1.0.0"

skip_transitive_dependency_licensing true

build do
    license :project_license

    # TODO too many things done here, should be split
    block do
        # Conf files
        if windows?
            ## this section creates the parallel `bin` directory structure for the Windows
            ## CF build pack.  None of the files created here will end up in the binary
            ## (MSI) distribution.
            cf_bin_root = "#{Omnibus::Config.source_dir()}/cf-root"
            cf_bin_root_bin = "#{cf_bin_root}/bin"
            cf_source_root = "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin"
            mkdir cf_bin_root_bin
            mkdir "#{cf_bin_root_bin}/agent"

            copy "#{cf_source_root}/agent/agent.exe", "#{cf_bin_root_bin}"
            copy "#{cf_source_root}/agent/libdatadog-agent-three.dll", "#{cf_bin_root_bin}"
            if with_python_runtime? "2"
                copy "#{cf_source_root}/agent/libdatadog-agent-two.dll", "#{cf_bin_root_bin}"
            end

            copy "#{cf_source_root}/agent/process-agent.exe", "#{cf_bin_root_bin}/agent"
            copy "#{cf_source_root}/agent/trace-agent.exe", "#{cf_bin_root_bin}/agent"
        end
    end
end
