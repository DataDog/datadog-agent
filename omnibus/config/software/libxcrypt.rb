# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2022-present Datadog, Inc.

require './lib/autotools.rb'

name "libxcrypt"
default_version "4.4.28"

version "4.4.28" do
    source sha256: "9e936811f9fad11dbca33ca19bd97c55c52eb3ca15901f27ade046cc79e69e87"
end

ship_source_offer true

source url: "https://github.com/besser82/libxcrypt/releases/download/v#{version}/libxcrypt-#{version}.tar.xz",
       extract: :seven_zip

relative_path "libxcrypt-#{version}"

build do
    license "LGPL-2.1"
    license_file "./COPYING.lib"

    # This builds libcrypt.so.2
    # To build libcrypt.so.1, the --disable-obsolete-api option
    # needs to be removed.
    autotools_params = {
      :configure_opts => ["--disable-obsolete-api"]
    }

    if redhat? && !arm? && ohai['platform_version'].to_i == 6
        # On the CentOS 6 builder, use gcc 4.9.2 in the devtoolset-3 env,
        # and ignore sign conversion warnings.
        autotools_params[:CC] = "/opt/rh/devtoolset-3/root/usr/bin/gcc"
        autotools_params[:CPP] = "/opt/rh/devtoolset-3/root/usr/bin/cpp"
        autotools_params[:CFLAGS] = " -Wno-sign-conversion"
    end

    build_with_autotools(autotools_params)
end
