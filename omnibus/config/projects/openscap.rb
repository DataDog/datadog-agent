# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require "./lib/ostools.rb"

name 'openscap'
package_name 'openscap'
license "Apache-2.0"
license_file "../LICENSE"

homepage 'https://www.open-scap.org'

INSTALL_DIR = '/opt/datadog-agent'
install_dir INSTALL_DIR

maintainer 'Datadog Packages <package@datadoghq.com>'

# build_version is computed by an invoke command/function.
# We can't call it directly from there, we pass it through the environment instead.
build_version '1.0.0'

build_iteration 1

description 'OpenSCAP'

# ------------------------------------
# Dependencies
# ------------------------------------

dependency 'openscap'

if linux? or windows?
  # the stripper will drop the symbols in a `.debug` folder in the installdir
  # we want to make sure that directory is not in the main build, while present
  # in the debug package.
  strip_build true
  debug_path ".debug"  # the strip symbols will be in here
end
