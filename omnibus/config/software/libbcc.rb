# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

name 'bcc'
# default_version 'v0.12.0'

dependency 'libelf'

# dependency 'libelf'
# # dependency 'libc++'

# source git: 'https://github.com/iovisor/bcc.git'

# relative_path 'bcc'

build do
  command "#{ENV['S3_CP_CMD']} #{ENV['S3_ARTIFACTS_URI']}/libbcc-amd64.tar.xz /tmp/libbcc.tar.xz"
  command 'tar -xvf /tmp/libbcc.tar.xz -C /opt/datadog-agent/embedded'
  # command "cmake . -DCMAKE_INSTALL_PREFIX=#{install_dir}/embedded -DCMAKE_EXE_LINKER_FLAGS='-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib -ltinfo' -DCMAKE_SHARED_LINKER_FLAGS='-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib -ltinfo'"
  # make "-j #{workers}"
  # make 'install'
end
