# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'db4'
default_version '4.8.30'

version '4.8.30' do
  source url: 'https://launchpad.net/ubuntu/+archive/primary/+sourcefiles/db4.8/4.8.30-11ubuntu1.1/db4.8_4.8.30.orig.tar.gz',
         sha512: 'd1a3c52b0ab54ae3fd6792e6396c9f74d25f36b2eb9e853b67ef9c872508a58c784c7818108d06d184f59601b70cc877916e67dfea6f0ee1ca2b07468c1041f1'
end

relative_path "db-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_options = []
  configure(*configure_options, bin: "../dist/configure", env: env, cwd: "#{project_dir}/build_unix")

  make "-j #{workers}", env: env, cwd: "#{project_dir}/build_unix"
  make "install", env: env, cwd: "#{project_dir}/build_unix"
end

