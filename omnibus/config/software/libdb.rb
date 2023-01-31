# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'libdb'
default_version '5.3.28'

dependency "config_guess"

version("5.3.28") { source sha256: "e0a992d740709892e81f9d93f06daf305cf73fb81b545afe72478043172c3628" }

source url: "https://github.com/berkeleydb/libdb/releases/download/v#{version}/db-#{version}.tar.gz"

relative_path "db-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  update_config_guess(target: "dist")

  configure_options = ["--disable-static"]
  configure(*configure_options, bin: "../dist/configure", env: env, cwd: "#{project_dir}/build_unix")

  make "-j #{workers}", env: env, cwd: "#{project_dir}/build_unix"
  make "install", env: env, cwd: "#{project_dir}/build_unix"
  make "uninstall_docs", env: env, cwd: "#{project_dir}/build_unix"
end

