# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "datadog-agent-finalize-licenses"
description "steps required to remove licenses"
skip_transitive_dependency_licensing true
always_build true

build do
  # compress separately each license file in the install dir
  Dir.glob("#{install_dir}/LICENSES/*").each do |license_file|
    command "tar -czf #{license_file}.tar.gz #{license_file}"
    delete "#{license_file}"
  end
end
