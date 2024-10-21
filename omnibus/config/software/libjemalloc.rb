# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2022-present Datadog, Inc.

name "libjemalloc"
default_version "5.3.0"

version "5.3.0" do
  source sha256: "ef6f74fd45e95ee4ef7f9e19ebe5b075ca6b7fbe0140612b2a161abafb7ee179"
end

ship_source_offer true

source url:"https://github.com/jemalloc/jemalloc/archive/refs/tags/#{version}.tar.gz",
       extract: :seven_zip

relative_path "jemalloc-#{version}"

build do
    license "BSD-2-Clause"
    license_file "./COPYING"

    env = with_standard_compiler_flags

    command "autoconf"

    # This builds libjemalloc.so.2
    configure_options = [
      "--disable-debug",
      "--disable-stats",
      "--disable-fill",
      "--disable-prof",
      "--disable-static",
    ]
    configure(*configure_options, env: env)
    command "make -j #{workers}", env: env
    command "make install_lib_shared"
end
