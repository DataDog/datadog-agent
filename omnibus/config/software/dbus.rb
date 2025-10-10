# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "dbus"
default_version "1.16.2"

dependency "libtool"
dependency "systemd"

license "AFL-2.1"
license_file "LICENSES/AFL-2.1.txt"
skip_transitive_dependency_licensing true

version("1.16.2") { source sha256: "0ba2a1a4b16afe7bceb2c07e9ce99a8c2c3508e5dec290dbb643384bd6beb7e2" }

source url: "https://dbus.freedesktop.org/releases/dbus/dbus-#{version}.tar.xz"

relative_path "dbus-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  meson_options = [
    "-Dmessage_bus=false",
    "-Ddoxygen_docs=disabled",
    "-Dducktype_docs=disabled",
    "-Dxml_docs=disabled",
    "-Dstats=false",
    "-Dtools=false",
    "-Dmodular_tests=disabled",
    "--prefix=#{install_dir}/embedded",
    "-Dbuildtype=release",
  ]

  # meson requires a dedicated build folder
  meson_build_dir = "#{project_dir}/build"
  command "mkdir #{meson_build_dir}", env: env
  command "meson setup " + meson_options.join(' ').strip + " ..", cwd: meson_build_dir
  command "ninja install", env: env, cwd: meson_build_dir

  # Remove dbus tools.
  delete "#{install_dir}/embedded/bin/dbus-*"
end
