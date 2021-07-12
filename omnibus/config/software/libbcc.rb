# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'bcc'

dependency 'libelf'

build do
  if ENV.has_key?('LIBBCC_TARBALL') and not ENV['LIBBCC_TARBALL'].empty?
    command "tar -xvf #{ENV['LIBBCC_TARBALL']} -C #{install_dir}/embedded"
  end
end
