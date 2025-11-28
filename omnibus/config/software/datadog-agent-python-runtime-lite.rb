# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'

name 'datadog-agent-python-runtime-lite'

license "Apache-2.0"
license_file "../LICENSE"

# This software packages Python runtime and debugging tools for lite flavor
dependency 'python3'
dependency 'pympler'

build do
  # For lite flavor, create separate Python runtime package
  if ENV['AGENT_FLAVOR'] == 'lite'
    python_runtime_dir = "#{install_dir}/python-runtime"
    mkdir python_runtime_dir
    
    # Copy Python runtime to separate directory
    if windows_target?
      copy "#{install_dir}/embedded3/*", "#{python_runtime_dir}/"
    else
      copy "#{install_dir}/embedded/*", "#{python_runtime_dir}/"
    end
    
    # Create marker file to indicate this is a lite Python runtime package
    command "echo 'lite' > #{python_runtime_dir}/AGENT_FLAVOR"
    command "echo 'python-runtime' > #{python_runtime_dir}/COMPONENT_TYPE"
    
    # Set proper permissions
    if windows_target?
      command "icacls #{windows_safe_path(python_runtime_dir)} /T /Q /C /RESET"
    else
      command "find #{python_runtime_dir} -type f -exec chmod 644 {} +"
      command "find #{python_runtime_dir} -type d -exec chmod 755 {} +"
    end
  end
end