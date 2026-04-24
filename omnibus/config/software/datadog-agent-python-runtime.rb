# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'

name 'datadog-agent-python-runtime'

license "Apache-2.0"
license_file "../LICENSE"

# Build Python interpreter and all integrations into the install dir.
# The resulting embedded/ directory gets packaged as an OCI artifact and,
# at install time, symlinked into the agent's embedded/ tree by the
# installer post-install hook (see pkg/fleet/installer/packages/
# datadog_agent_python_runtime_linux.go).
dependency 'python3'
dependency 'datadog-agent-integrations-py3'

# Memory profiler used by the `status py` agent subcommand. Ships with the
# runtime so it is available whenever Python checks are available.
dependency 'pympler'

build do
  # Copy the python-scripts (pre.py / post.py for custom integration
  # preservation) into the runtime package. These are normally placed
  # by datadog-agent.rb; for the standalone runtime we do it here so
  # custom integration save/restore works without the full agent build.
  python_scripts_src = "#{Omnibus::Config.project_root}/../omnibus/python-scripts"
  if File.exist?(python_scripts_src)
    mkdir "#{install_dir}/python-scripts"
    Dir.glob("#{python_scripts_src}/*").each do |file|
      unless File.basename(file).end_with?('_tests.py')
        copy file, "#{install_dir}/python-scripts"
      end
    end
  end
end
