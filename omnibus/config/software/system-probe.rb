# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

name 'system-probe'

dependency 'libbcc'

build do
  command "#{ENV['S3_CP_CMD']} #{ENV['S3_ARTIFACTS_URI']}/system-probe.#{ENV['PACKAGE_ARCH']} #{install_dir}/embedded/bin/system-probe"
end
