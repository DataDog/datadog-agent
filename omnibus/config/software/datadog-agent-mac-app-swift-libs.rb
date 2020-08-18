# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

name 'datadog-agent-mac-app-swift-libs'

default_version "1.0.0"

source :url => "https://dd-agent-omnibus.s3.amazonaws.com/swift-libs-#{version}.tar.gz",
       :sha256 => "d0e8f29d4f51df934f63ccecf83291ead509c03c2809db5653ac754734839653"

whitelist_file "embedded/lib/libswift.*\.dylib"

build do
    app_temp_dir = "#{install_dir}/Datadog Agent.app/Contents"
    mkdir "#{app_temp_dir}/Frameworks"

    # Add swift runtime libs in Frameworks (same thing a full xcode build would do).
    # They are needed for the gui to run on MacOS 10.14.3 and lower
    copy "*.dylib", "#{app_temp_dir}/Frameworks/"
end