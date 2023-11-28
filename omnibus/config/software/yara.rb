# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "yara"
default_version "4.3.2"

license "BSD-3-Clause"
license_file "COPYING"
skip_transitive_dependency_licensing true

version("4.3.2") { source sha256: "a9587a813dc00ac8cdcfd6646d7f1c172f730cda8046ce849dfea7d3f6600b15" }

ship_source_offer true

source url: "https://github.com/VirusTotal/yara/archive/refs/tags/v#{version}.tar.gz"

relative_path "#{name}-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"

  command "sed -i /EXTRA_test_rules_DEPENDENCIES/d Makefile.am"
  command "./bootstrap.sh "

  configure_options = [
    " --disable-static",
    " --enable-magic",
    " --enable-macho",
    " --enable-dex"
  ]
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env
end
