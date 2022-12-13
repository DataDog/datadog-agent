# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'libdb'
default_version '5.3.28'

version '5.3.28' do
  source url: 'https://github.com/berkeleydb/libdb/releases/download/v5.3.28/db-5.3.28.tar.gz',
         sha512: 'e91bbe550fc147a8be7e69ade86fdb7066453814971b2b0223f7d17712bd029a8eff5b2b6b238042ff6ec1ffa6879d44cb95c5645a922fee305c26c3eeaee090'
end

relative_path "db-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)


  configure_options = [
  ]
  configure(*configure_options, bin: "../dist/configure", env: env, cwd: "#{project_dir}/build_unix")

  make "-j #{workers}", env: env, cwd: "#{project_dir}/build_unix"
  make "install", env: env, cwd: "#{project_dir}/build_unix"
end

