# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "datadog-windows-apminject"
# at this moment,builds are stored by branch name.  Will need to correct at some point


default_version "master"
#
# this should only ever be included by a windows build.
if ohai["platform"] == "windows"
    driverpath = ENV['WINDOWS_APMINJECT_MODULE']
    driverver = ENV['WINDOWS_APMINJECT_VERSION']
    drivermsmsha = ENV['WINDOWS_APMINJECT_SHASUM']

    source :url => "https://s3.amazonaws.com/dd-windowsfilter/builds/#{driverpath}/ddapminstall-#{driverver}.msm",
           :sha256 => "#{drivermsmsha}",
           :target_filename => "ddapminstall.msm"

    build do
        copy "ddapminstall.msm", "#{install_dir}/bin/agent/ddapminstall.msm"
    end

end