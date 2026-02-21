# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'json'

name 'datadog-agent-integrations-py3-lite'

license "BSD-3-Clause"
license_file "./LICENSE"

dependency 'datadog-agent-integrations-py3-dependencies'

python_version = "3.13"

relative_path 'integrations-core'
whitelist_file "embedded/lib/python#{python_version}/site-packages/.libsaerospike"
whitelist_file "embedded/lib/python#{python_version}/site-packages/aerospike.libs"
whitelist_file "embedded/lib/python#{python_version}/site-packages/psycopg_binary.libs"
whitelist_file "embedded/lib/python#{python_version}/site-packages/pymqi"

source git: 'https://github.com/DataDog/integrations-core.git'

integrations_core_version = ENV['INTEGRATIONS_CORE_VERSION']
if integrations_core_version.nil? || integrations_core_version.empty?
  integrations_core_version = 'master'
end
default_version integrations_core_version

build do
  # Set the PYTHONPATH and PYTHONHOME
  env = {
    "PYTHONPATH" => "#{install_dir}/embedded/lib/python#{python_version}/site-packages",
    "PYTHONHOME" => "#{install_dir}/embedded",
  }
  
  # For lite flavor, install integrations to separate directory
  integrations_dir = "#{install_dir}/python-integrations"
  mkdir integrations_dir
  mkdir "#{integrations_dir}/lib/python#{python_version}/site-packages"
  
  env["PYTHONPATH"] = "#{integrations_dir}/lib/python#{python_version}/site-packages"
  
  pip_install_args = "#{windows_safe_path(install_dir)}\\embedded3\\python.exe -m pip install --no-deps --no-binary=:all:" if windows_target?
  pip_install_args = "#{install_dir}/embedded/bin/pip3 install --no-deps --no-binary=:all:" unless windows_target?

  patch :source => 'create-regex-at-runtime.patch', :env => env if ohai['platform'] == "windows" && !arm64_target?

  # Get static requirements
  env_script_path = "#{windows_safe_path(install_dir)}/embedded3/Scripts/Activate.ps1"
  env_script_path = ". #{install_dir}/embedded/bin/activate" unless windows_target?

  requirements_file = 'omnibus/config/templates/datadog-agent-integrations-py3/static_requirements.txt'

  if windows_target?
    requirements_file = 'omnibus\\config\\templates\\datadog-agent-integrations-py3\\static_requirements.txt'
  end

  erb :dest => "#{Dir.pwd}/static_requirements.txt",
      :source => "#{windows_safe_path(Dir.pwd)}/#{requirements_file}.erb",
      :mode => 0755,
      :vars => { :python_version => python_version, :requirements => {}, :static_reqs_in_requirements => [] }

  # Use the static requirements file to install base requirements to python-integrations dir
  if windows_target?
    install_cmd = "#{pip_install_args} --target #{integrations_dir}\\lib\\python#{python_version}\\site-packages"
  else
    install_cmd = "#{pip_install_args} --target #{integrations_dir}/lib/python#{python_version}/site-packages"
  end

  command "#{install_cmd} -r static_requirements.txt", :env => env

  # Install integrations to separate directory
  # Use smaller core set for lite flavor
  core_integrations = %w[cpu disk io memory network uptime]
  
  core_integrations.each do |integration|
    if File.exist?("#{integration}/setup.py")
      command "#{install_cmd} ./#{integration}", :env => env, :cwd => "#{project_dir}"
    end
  end
  
  # Create marker file to indicate this is a lite integrations package
  command "echo 'lite' > #{integrations_dir}/AGENT_FLAVOR"
  
  # Set proper permissions
  if ohai['platform'] == "windows"
    command "icacls #{windows_safe_path(integrations_dir)} /T /Q /C /RESET"
  else
    command "find #{integrations_dir} -type f -exec chmod 644 {} +"
    command "find #{integrations_dir} -type d -exec chmod 755 {} +"
  end
end