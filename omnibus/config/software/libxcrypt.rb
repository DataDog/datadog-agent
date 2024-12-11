# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2022-present Datadog, Inc.

name "libxcrypt"
default_version "4.4.36"

version "4.4.36" do
    source sha256: "e5e1f4caee0a01de2aee26e3138807d6d3ca2b8e67287966d1fefd65e1fd8943"
end

ship_source_offer true

source url: "https://github.com/besser82/libxcrypt/releases/download/v#{version}/libxcrypt-#{version}.tar.xz",
       extract: :seven_zip

relative_path "libxcrypt-#{version}"

build do
    license "LGPL-2.1"
    license_file "./COPYING.lib"

    env = with_standard_compiler_flags

    # This builds libcrypt.so.2
    # To build libcrypt.so.1, the --disable-obsolete-api option
    # needs to be removed.
    configure_options = [
        "--disable-obsolete-api",
        "--disable-static",
        "--enable-shared",
    ]
    configure(*configure_options, env: env)
    command "make -j #{workers}", env: env
    command "make -j #{workers} install"
end
