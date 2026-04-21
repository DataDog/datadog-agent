# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
require "./lib/ostools.rb"
require "./lib/project_helpers.rb"
require "./lib/omnibus/packagers/tarball.rb"

name 'python-runtime'
package_name 'datadog-agent-python-runtime'

license "Apache-2.0"
license_file "../LICENSE"

third_party_licenses "../LICENSE-3rdparty.csv"

homepage 'http://www.datadoghq.com'

if ENV.has_key?("OMNIBUS_WORKERS_OVERRIDE")
  COMPRESSION_THREADS = ENV["OMNIBUS_WORKERS_OVERRIDE"].to_i
else
  COMPRESSION_THREADS = 1
end

if ENV.has_key?("DEPLOY_AGENT") && ENV["DEPLOY_AGENT"] == "true"
  COMPRESSION_LEVEL = 9
else
  COMPRESSION_LEVEL = 5
end

if ENV.has_key?("OMNIBUS_GIT_CACHE_DIR")
  Omnibus::Config.use_git_caching true
  Omnibus::Config.git_cache_dir ENV["OMNIBUS_GIT_CACHE_DIR"]
end

# Install dir matches the agent's layout so that, when the runtime is
# installed as an OCI package under /opt/datadog-packages/
# datadog-agent-python-runtime/<version>/, its embedded/ tree mirrors
# the agent's embedded/ tree and can be symlinked across by the
# installer post-install hook.
INSTALL_DIR = ENV["INSTALL_DIR"] || '/opt/datadog-agent'
install_dir INSTALL_DIR

json_manifest_path File.join(install_dir, "version-manifest.python-runtime.json")
text_manifest_path File.join(install_dir, "version-manifest.python-runtime.txt")

# build_version is computed by an invoke command/function and passed through
# the environment, same pattern as the other omnibus projects.
build_version ENV['PACKAGE_VERSION']

build_iteration 1

description 'Python runtime and integrations for the Datadog Agent'

maintainer 'Datadog Packages <package@datadoghq.com>'

# ------------------------------------
# Dependencies
# ------------------------------------
dependency 'datadog-agent-python-runtime'

exclude '\.git*'
exclude 'bundler\/git'

# The Python runtime is currently shipped only as an OCI artifact. Skip
# the deb/rpm/msi packagers; the .tar.xz produced by the :xz packager is
# what feeds the `datadog-package create` step in CI.
package :deb do
  skip_packager true
end

package :rpm do
  skip_packager true
end

package :msi do
  skip_packager true
end

package :xz do
  skip_packager (ENV["SKIP_PKG_COMPRESSION"] == "true")
  compression_threads COMPRESSION_THREADS
  compression_level COMPRESSION_LEVEL
end

package :tarball do
  skip_packager !(ENV["SKIP_PKG_COMPRESSION"] == "true")
end
