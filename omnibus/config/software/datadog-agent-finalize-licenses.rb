# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "datadog-agent-finalize-licenses"
description "steps required to remove licenses"

always_build true

build do
  if linux_target?
    command "tar -czf #{install_dir}/LICENSES.tar.gz #{install_dir}/LICENSES"
    delete "#{install_dir}/LICENSES"
  end
end
