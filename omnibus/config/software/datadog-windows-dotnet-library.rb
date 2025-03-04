# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "datadog-windows-dotnet-library"

# at this moment,builds are stored by branch name.  Will need to correct at some point
default_version "master"

# this should only ever be included by a windows build.
if ohai["platform"] == "windows"
    oci_version = ENV['WINDOWS_DOTNET_LIBRARY_VERSION']
    build do
        command "crane pull --platform windows/amd64 --format oci install.datad0g.com/apm-library-dotnet-package:#{oci_version} #{install_dir}/bin/agent/apm-library-dotnet-package"
    end

end
