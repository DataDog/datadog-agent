# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

require "./lib/ostools.rb"

name 'datadog-cluster-agent'
package_name 'datadog-cluster-agent'

homepage 'http://www.datadoghq.com'

if ohai['platform'] == "windows"
  # Note: this is the path used by Omnibus to build the agent, the final install
  # dir will be determined by the Windows installer. This path must not contain
  # spaces because Omnibus doesn't quote the Git commands it launches.
  install_dir "C:/opt/datadog-cluster-agent/"
  maintainer 'Datadog Inc.' # Windows doesn't want our e-mail address :(
else
  install_dir '/opt/datadog-cluster-agent'
  maintainer 'Datadog Packages <package@datadoghq.com>'
end

build_version do
  source :git
  output_format :dd_agent_format
end

build_iteration 1

description 'Datadog Cluster Agent
 The Datadog Cluster Agent is a lightweight process that is designed as a add-on for
 the classic Datadog Monitoring Agent to provide more insight on metrics and events
 in Kubernetes environments.
 .
 This package installs and runs the Cluster Agent daemon, which queues and
 forwards events as well as collects metadata from the Kubernetes API server.
 It also exposes various API endpoints for Datadog Monitoring Agents to get more
 metadata on the metrics collected by their Kubernetes integration.
 .
 See http://www.datadoghq.com/ for more information
'

# ------------------------------------
# Generic package information
# ------------------------------------

# .deb specific flags
package :deb do
  vendor 'Datadog <package@datadoghq.com>'
  epoch 1
  license 'Apache License Version 2.0'
  section 'utils'
  priority 'extra'
end

# .rpm specific flags
package :rpm do
  vendor 'Datadog <package@datadoghq.com>'
  epoch 1
  dist_tag ''
  license 'Apache License Version 2.0'
  category 'System Environment/Daemons'
  priority 'extra'
  if ENV.has_key?('RPM_SIGNING_PASSPHRASE') and not ENV['RPM_SIGNING_PASSPHRASE'].empty?
    signing_passphrase "#{ENV['RPM_SIGNING_PASSPHRASE']}"
  end
end

# OSX .pkg specific flags
package :pkg do
  identifier 'com.datadoghq.cluster-agent'
  unless ENV['SKIP_SIGN_MAC'] == 'true'
    signing_identity 'Developer ID Installer: Datadog, Inc. (JKFCB4CN7C)'
  end
end
compress :dmg do
  window_bounds '200, 200, 750, 600'
  pkg_position '10, 10'
end

# ------------------------------------
# Dependencies
# ------------------------------------

# creates required build directories
dependency 'datadog-agent-prepare'

# version manifest file
dependency 'version-manifest'

# Datadog Cluster agent
dependency 'datadog-cluster-agent'

# this dependency puts few files out of the omnibus install dir and move them
# in the final destination. This way such files will be listed in the packages
# manifest and owned by the package manager. This is the only point in the build
# process where we operate outside the omnibus install dir, thus the need of
# the `extra_package_file` directive.
# This must be the last dependency in the project.

dependency 'datadog-cluster-agent-finalize'

if linux?
  extra_package_file '/etc/init/datadog-cluster-agent.conf'
  extra_package_file '/lib/systemd/system/datadog-cluster-agent.service'
  extra_package_file '/etc/datadog-cluster-agent/'
end

exclude '\.git*'
exclude 'bundler\/git'
