# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'json'

name 'datadog-agent-integrations-py3'

license "BSD-3-Clause"
license_file "./LICENSE"

dependency 'datadog-agent-integrations-py3-dependencies'

relative_path 'integrations-core'
whitelist_file "embedded/lib/python3.11/site-packages/.libsaerospike"
whitelist_file "embedded/lib/python3.11/site-packages/aerospike.libs"
whitelist_file "embedded/lib/python3.11/site-packages/psycopg2"
whitelist_file "embedded/lib/python3.11/site-packages/pymqi"

source git: 'https://github.com/DataDog/integrations-core.git'

always_build true

integrations_core_version = ENV['INTEGRATIONS_CORE_VERSION']
if integrations_core_version.nil? || integrations_core_version.empty?
  integrations_core_version = 'master'
end
default_version integrations_core_version

# folder names containing integrations from -core that won't be packaged with the Agent
excluded_folders = [
  'datadog_checks_base',           # namespacing package for wheels (NOT AN INTEGRATION)
  'datadog_checks_dev',            # Development package, (NOT AN INTEGRATION)
  'datadog_checks_tests_helper',   # Testing and Development package, (NOT AN INTEGRATION)
  'docker_daemon',                 # Agent v5 only
]

if osx_target?
  # Temporarily exclude Aerospike until builder supports new dependency
  excluded_folders.push('aerospike')
  excluded_folders.push('teradata')
end

if arm_target?
  # This doesn't build on ARM
  excluded_folders.push('ibm_ace')
  excluded_folders.push('ibm_mq')
end

final_constraints_file = 'final_constraints-py3.txt'
agent_requirements_file = 'agent_requirements-py3.txt'
filtered_agent_requirements_in = 'agent_requirements-py3.in'
agent_requirements_in = 'agent_requirements.in'

build do
  # The dir for confs
  if osx_target?
    conf_dir = "#{install_dir}/etc/conf.d"
  else
    conf_dir = "#{install_dir}/etc/datadog-agent/conf.d"
  end
  mkdir conf_dir

  # aliases for pip
  if windows_target?
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  else
    python = "#{install_dir}/embedded/bin/python3"
  end

  # If a python_mirror is set, it is set in a pip config file so that we do not leak the token in the CI output
  pip_config_file = ENV['PIP_CONFIG_FILE']
  pre_build_env = {
    "PIP_CONFIG_FILE" => "#{pip_config_file}"
  }

  # Install dependencies
  lockfile_name = case
    when linux_target?
      arm_target? ? "linux-aarch64_py3.txt" : "linux-x86_64_py3.txt"
    when osx_target?
      "macos-x86_64_py3.txt"
    when windows_target?
      "windows-x86_64_py3.txt"
  end
  lockfile = windows_safe_path(project_dir, ".deps", "resolved", lockfile_name)
  command "#{python} -m pip install --require-hashes --only-binary=:all: --no-deps -r #{lockfile}"

  # Prepare build env for integrations
  wheel_build_dir = windows_safe_path(project_dir, ".wheels")
  build_deps_dir = windows_safe_path(project_dir, ".build_deps")
  # We download build dependencies to make them available without an index when installing integrations
  command "#{python} -m pip download --dest #{build_deps_dir} hatchling==0.25.1", :env => pre_build_env
  command "#{python} -m pip download --dest #{build_deps_dir} setuptools==66.1.1", :env => pre_build_env # Version from ./setuptools3.rb
  build_env = {
    "PIP_FIND_LINKS" => build_deps_dir,
    "PIP_CONFIG_FILE" => pip_config_file,
  }

  # Install base and downloader packages
  cwd_base = windows_safe_path(project_dir, "datadog_checks_base")
  cwd_downloader = windows_safe_path(project_dir, "datadog_checks_downloader")
  command "#{python} -m pip wheel . --no-deps --no-index --wheel-dir=#{wheel_build_dir}", :env => build_env, :cwd => cwd_base
  command "#{python} -m pip install datadog_checks_base --no-deps --no-index --find-links=#{wheel_build_dir}"
  command "#{python} -m pip wheel . --no-deps --no-index --wheel-dir=#{wheel_build_dir}", :env => build_env, :cwd => cwd_downloader
  command "#{python} -m pip install datadog_checks_downloader --no-deps --no-index --find-links=#{wheel_build_dir}"

  #
  # Install Core integrations
  #

  # Create a constraint file after installing all the core dependencies and before any integration
  # This is then used as a constraint file by the integration command to avoid messing with the agent's python environment
  command "#{python} -m pip freeze > #{install_dir}/#{final_constraints_file}"

  if windows_target?
    cached_wheels_dir = "#{windows_safe_path(wheel_build_dir)}\\.cached"
  else
    cached_wheels_dir = "#{wheel_build_dir}/.cached"
  end

  block "Install integrations" do
    tasks_dir_in = windows_safe_path(Dir.pwd)
    # Collect integrations to install
    checks_to_install = (
      shellout! "inv agent.collect-integrations #{project_dir} 3 #{os} #{excluded_folders.join(',')}",
                :cwd => tasks_dir_in
    ).stdout.split()

    # Retrieving integrations from cache
    cache_bucket = ENV.fetch('INTEGRATION_WHEELS_CACHE_BUCKET', '')
    cache_branch = (shellout! "inv release.get-release-json-value base_branch", cwd: File.expand_path('..', tasks_dir_in)).stdout.strip
    # On windows, `aws` actually executes Ruby's AWS SDK, but we want the Python one
    awscli = if windows_target? then '"c:\Program files\python311\scripts\aws"' else 'aws' end
    if cache_bucket != ''
      mkdir cached_wheels_dir
      shellout! "inv -e agent.get-integrations-from-cache " \
                "--python 3 --bucket #{cache_bucket} " \
                "--branch #{cache_branch || 'main'} " \
                "--integrations-dir #{windows_safe_path(project_dir)} " \
                "--target-dir #{cached_wheels_dir} " \
                "--integrations #{checks_to_install.join(',')} " \
                "--awscli #{awscli}",
                :cwd => tasks_dir_in

      # install all wheels from cache in one pip invocation to speed things up
      if windows_target?
        shellout! "#{python} -m pip install --no-deps --no-index " \
                  " --find-links #{windows_safe_path(cached_wheels_dir)} -r #{windows_safe_path(cached_wheels_dir)}\\found.txt"
      else
        shellout! "#{python} -m pip install --no-deps --no-index " \
                  "--find-links #{cached_wheels_dir} -r #{cached_wheels_dir}/found.txt"
      end
    end

    # get list of integration wheels already installed from cache
    installed_list = Array.new
    if cache_bucket != ''
      installed_out = `#{python} -m pip list --format json`
      if $?.exitstatus == 0
        installed = JSON.parse(installed_out)
        installed.each do |package|
          package.each do |key, value|
            if key == "name" && value.start_with?("datadog-")
              installed_list.push(value["datadog-".length..-1])
            end
          end
        end
      else
        raise "Failed to list pip installed packages"
      end
    end

    checks_to_install.each do |check|
      check_dir = File.join(project_dir, check)
      check_conf_dir = "#{conf_dir}/#{check}.d"

      # For each conf file, if it already exists, that means the `datadog-agent` software def
      # wrote it first. In that case, since the agent's confs take precedence, skip the conf
      conf_files = ["conf.yaml.example", "conf.yaml.default", "metrics.yaml", "auto_conf.yaml"]
      conf_files.each do |filename|
        src = windows_safe_path(check_dir,"datadog_checks", check, "data", filename)
        dest = check_conf_dir
        if File.exist?(src) and !File.exist?(windows_safe_path(dest, filename))
          FileUtils.mkdir_p(dest)
          FileUtils.cp_r(src, dest)
        end
      end

      # Copy SNMP profiles
      profile_folders = ['profiles', 'default_profiles']
      profile_folders.each do |profile_folder|
        folder_path = "#{check_dir}/datadog_checks/#{check}/data/#{profile_folder}"
        if File.exist? folder_path
          FileUtils.cp_r folder_path, "#{check_conf_dir}/"
        end
      end

      # pip < 21.2 replace underscores by dashes in package names per https://pip.pypa.io/en/stable/news/#v21-2
      # whether or not this might switch back in the future is not guaranteed, so we check for both name
      # with dashes and underscores
      if installed_list.include?(check) || installed_list.include?(check.gsub('_', '-'))
        next
      end

      if windows_target?
        shellout! "#{python} -m pip wheel . --no-deps --no-index --wheel-dir=#{wheel_build_dir}", :env => build_env, :cwd => "#{windows_safe_path(project_dir)}\\#{check}"
      else
        shellout! "#{python} -m pip wheel . --no-deps --no-index --wheel-dir=#{wheel_build_dir}", :env => build_env, :cwd => "#{project_dir}/#{check}"
      end
      shellout! "#{python} -m pip install datadog-#{check} --no-deps --no-index --find-links=#{wheel_build_dir}"
      if cache_bucket != '' && ENV.fetch('INTEGRATION_WHEELS_SKIP_CACHE_UPLOAD', '') == '' && cache_branch != nil
        shellout! "inv -e agent.upload-integration-to-cache " \
                  "--python 3 --bucket #{cache_bucket} " \
                  "--branch #{cache_branch} " \
                  "--integrations-dir #{windows_safe_path(project_dir)} " \
                  "--build-dir #{wheel_build_dir} " \
                  "--integration #{check} " \
                  "--awscli #{awscli}",
                  :cwd => tasks_dir_in
      end
    end
  end

  # Patch applies to only one file: set it explicitly as a target, no need for -p
  if windows_target?
    patch :source => "remove-maxfile-maxpath-psutil.patch", :target => "#{python_3_embedded}/Lib/site-packages/psutil/__init__.py"
  else
    patch :source => "remove-maxfile-maxpath-psutil.patch", :target => "#{install_dir}/embedded/lib/python3.11/site-packages/psutil/__init__.py"
  end

  # Run pip check to make sure the agent's python environment is clean, all the dependencies are compatible
  command "#{python} -m pip check"

  # Removing tests that don't need to be shipped in the embedded folder
  if windows_target?
    delete "#{python_3_embedded}/Lib/site-packages/Cryptodome/SelfTest/"
  else
    delete "#{install_dir}/embedded/lib/python3.11/site-packages/Cryptodome/SelfTest/"
  end

  # Ship `requirements-agent-release.txt` file containing the versions of every check shipped with the agent
  # Used by the `datadog-agent integration` command to prevent downgrading a check to a version
  # older than the one shipped in the agent
  copy "#{project_dir}/requirements-agent-release.txt", "#{install_dir}/"
end
