# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "dbus"
default_version "1.15.4"

dependency "expat"
dependency "libtool"
dependency "pkg-config"

license "AFL-2.1"
license_file "LICENSES/AFL-2.1.txt"
skip_transitive_dependency_licensing true

version("1.15.4") { source sha256: "bfe53d9e54a4977ec344928521b031af2e97cf78aea58f5d8e2b85ea0a80028b" }

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
