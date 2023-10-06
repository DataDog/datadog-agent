# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "dbus"
default_version "1.14.10"

dependency "expat"
dependency "libtool"
dependency "pkg-config"

license "AFL-2.1"
license_file "LICENSES/AFL-2.1.txt"
skip_transitive_dependency_licensing true

version("1.14.10") { source sha256: "ba1f21d2bd9d339da2d4aa8780c09df32fea87998b73da24f49ab9df1e36a50f" }

source url: "https://dbus.freedesktop.org/releases/dbus/dbus-#{version}.tar.xz"

relative_path "dbus-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_options = [
    "--prefix=#{install_dir}/embedded",
    "--disable-static",
    "--disable-doxygen-docs",
    "--disable-ducktype-docs",
    "--disable-xml-docs",
    "--disable-stats",
    "--disable-tests",
    "--without-x",
  ]

  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env

  # Remove dbus tools.
  delete "#{install_dir}/embedded/bin/dbus-*"
end
