# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

name 'system-probe'

if ENV.has_key?('WITH_BCC')
  dependency 'libbcc'
end

build do
  if ENV.has_key?('SYSTEM_PROBE_BIN') and not ENV['SYSTEM_PROBE_BIN'].empty?
    copy ENV['SYSTEM_PROBE_BIN'], "#{install_dir}/embedded/bin/system-probe"
  end
end
