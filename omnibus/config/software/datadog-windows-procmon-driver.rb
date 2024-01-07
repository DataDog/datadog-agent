# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "datadog-windows-procmon-driver"
# at this moment,builds are stored by branch name.  Will need to correct at some point


default_version "master"
#
# this should only ever be included by a windows build.
if ohai["platform"] == "windows"
    driverpath = ENV['WINDOWS_DDPROCMON_DRIVER']
    driverver = ENV['WINDOWS_DDPROCMON_VERSION']
    drivermsmsha = ENV['WINDOWS_DDPROCMON_SHASUM']
    
    source :url => "https://dd-windowsfilter.s3.amazonaws.com/builds/#{driverpath}/ddprocmoninstall-#{driverver}.msm",
           :sha256 => "#{drivermsmsha}",
           :target_filename => "DDPROCMON.msm"

    build do
        copy "DDPROCMON.msm", "#{install_dir}/bin/agent/DDPROCMON.msm"
    end

end