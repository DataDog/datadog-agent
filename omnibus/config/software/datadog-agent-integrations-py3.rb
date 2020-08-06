# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

require './lib/ostools.rb'
require 'json'

name 'datadog-agent-integrations-py3'

dependency 'datadog-agent'
dependency 'pip3'

if arm?
  # psycopg2 doesn't come with pre-built wheel on the arm architecture.
  # to compile from source, it requires the `pg_config` executable present on the $PATH
  dependency 'postgresql'
  # same with libffi to build the cffi wheel
  dependency 'libffi'
  # same with libxml2 and libxslt to build the lxml wheel
  dependency 'libxml2'
  dependency 'libxslt'
end

if osx?
  dependency 'unixodbc'
end

if linux?
  # add nfsiostat script
  dependency 'unixodbc'
  dependency 'freetds'  # needed for SQL Server integration
  dependency 'nfsiostat'
  # need kerberos for hdfs
  dependency 'libkrb5'

  unless suse? || arm?
    dependency 'aerospike-py3'
  end
end

relative_path 'integrations-core'
whitelist_file "embedded/lib/python3.8/site-packages/psycopg2"
whitelist_file "embedded/lib/python3.8/site-packages/pymqi"

source git: 'https://github.com/DataDog/integrations-core.git'

integrations_core_version = ENV['INTEGRATIONS_CORE_VERSION']
if integrations_core_version.nil? || integrations_core_version.empty?
  integrations_core_version = 'master'
end
default_version integrations_core_version

# folder names containing integrations from -core that won't be packaged with the Agent
blacklist_folders = [
  'datadog_checks_base',           # namespacing package for wheels (NOT AN INTEGRATION)
  'datadog_checks_dev',            # Development package, (NOT AN INTEGRATION)
  'datadog_checks_tests_helper',   # Testing and Development package, (NOT AN INTEGRATION)
  'agent_metrics',
  'docker_daemon',
  'kubernetes',
  'ntp',                           # provided as a go check by the core agent
  # Python 2-only
  'tokumx',
]

# package names of dependencies that won't be added to the Agent Python environment
blacklist_packages = Array.new

# We build these manually
blacklist_packages.push(/^aerospike==/)

if suse?
  blacklist_folders.push('aerospike')  # Temporarily blacklist Aerospike until builder supports new dependency
end

if osx?
  # Blacklist lxml as it fails MacOS notarization: the etree.cpython-37m-darwin.so and objectify.cpython-37m-darwin.so
  # binaries were built with a MacOS SDK lower than 10.9.
  # This can be removed once a new lxml version with binaries built with a newer SDK is available.
  blacklist_packages.push(/^lxml==/)
  # Blacklist ibm_was, which depends on lxml
  blacklist_folders.push('ibm_was')

  # Blacklist aerospike, new version 3.10 is not supported on MacOS yet
  blacklist_folders.push('aerospike')
end

if arm?
  # These two checks don't build on ARM
  blacklist_folders.push('aerospike')
  blacklist_folders.push('ibm_mq')
  blacklist_packages.push(/^pymqi==/)
end

# _64_bit checks the kernel arch.  On windows, the builder is 64 bit
# even when doing a 32 bit build.  Do a specific check for the 32 bit
# build
if arm? || !_64_bit? || (windows? && windows_arch_i386?)
  blacklist_packages.push(/^orjson==/)
end

final_constraints_file = 'final_constraints-py3.txt'
agent_requirements_file = 'agent_requirements-py3.txt'
filtered_agent_requirements_in = 'agent_requirements-py3.in'
agent_requirements_in = 'agent_requirements.in'

build do
  # The dir for confs
  if osx?
    conf_dir = "#{install_dir}/etc/conf.d"
  else
    conf_dir = "#{install_dir}/etc/datadog-agent/conf.d"
  end
  mkdir conf_dir

  # aliases for pip
  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
    python = "#{install_dir}/embedded/bin/python3"
  end

  # Install the checks along with their dependencies
  block do
    #
    # Prepare the build env, these dependencies are only needed to build and
    # install the core integrations.
    #
    command "#{pip} install wheel==0.34.1"
    command "#{pip} install pip-tools==4.2.0"
    uninstall_buildtime_deps = ['rtloader', 'click', 'first', 'pip-tools']
    nix_build_env = {
      "CFLAGS" => "-I#{install_dir}/embedded/include -I/opt/mqm/inc",
      "CXXFLAGS" => "-I#{install_dir}/embedded/include -I/opt/mqm/inc",
      "LDFLAGS" => "-L#{install_dir}/embedded/lib -L/opt/mqm/lib64 -L/opt/mqm/lib",
      "LD_RUN_PATH" => "#{install_dir}/embedded/lib -L/opt/mqm/lib64 -L/opt/mqm/lib",
      "PATH" => "#{install_dir}/embedded/bin:#{ENV['PATH']}",
    }

    #
    # Prepare the requirements file containing ALL the dependencies needed by
    # any integration. This will provide the "static Python environment" of the Agent.
    # We don't use the .in file provided by the base check directly because we
    # want to filter out things before installing.
    #
    if windows?
      static_reqs_in_file = "#{windows_safe_path(project_dir)}\\datadog_checks_base\\datadog_checks\\base\\data\\#{agent_requirements_in}"
      static_reqs_out_file = "#{windows_safe_path(project_dir)}\\#{filtered_agent_requirements_in}"
    else
      static_reqs_in_file = "#{project_dir}/datadog_checks_base/datadog_checks/base/data/#{agent_requirements_in}"
      static_reqs_out_file = "#{project_dir}/#{filtered_agent_requirements_in}"
    end

    # Remove any blacklisted requirements from the static-environment req file
    requirements = Array.new
    File.open("#{static_reqs_in_file}", 'r+').readlines().each do |line|
      blacklist_flag = false
      blacklist_packages.each do |blacklist_regex|
        re = Regexp.new(blacklist_regex).freeze
        if re.match line
          blacklist_flag = true
        end
      end

      if !blacklist_flag
        requirements.push(line)
      end
    end

    # Adding pympler for memory debug purposes
    requirements.push("pympler==0.7")

    # Render the filtered requirements file
    erb source: "static_requirements.txt.erb",
        dest: "#{static_reqs_out_file}",
        mode: 0640,
        vars: { requirements: requirements }

    # Use pip-compile to create the final requirements file. Notice when we invoke `pip` through `python -m pip <...>`,
    # there's no need to refer to `pip`, the interpreter will pick the right script.
    if windows?
      command "#{python} -m pip install --no-deps  #{windows_safe_path(project_dir)}\\datadog_checks_base"
      command "#{python} -m pip install --no-deps  #{windows_safe_path(project_dir)}\\datadog_checks_downloader --install-option=\"--install-scripts=#{windows_safe_path(install_dir)}/bin\""
      command "#{python} -m piptools compile --generate-hashes --output-file #{windows_safe_path(install_dir)}\\#{agent_requirements_file} #{static_reqs_out_file}"
    else
      command "#{pip} install --no-deps .", :env => nix_build_env, :cwd => "#{project_dir}/datadog_checks_base"
      command "#{pip} install --no-deps .", :env => nix_build_env, :cwd => "#{project_dir}/datadog_checks_downloader"
      command "#{python} -m piptools compile --generate-hashes --output-file #{install_dir}/#{agent_requirements_file} #{static_reqs_out_file}", :env => nix_build_env
    end

    # From now on we don't need piptools anymore, uninstall its deps so we don't include them in the final artifact
    uninstall_buildtime_deps.each do |dep|
      if windows?
        command "#{python} -m pip uninstall -y #{dep}"
      else
        command "#{pip} uninstall -y #{dep}"
      end
    end

    #
    # Install static-environment requirements that the Agent and all checks will use
    #
    if windows?
      command "#{python} -m pip install --no-deps --require-hashes -r #{windows_safe_path(install_dir)}\\#{agent_requirements_file}"
    else
      command "#{pip} install --no-deps --require-hashes -r #{install_dir}/#{agent_requirements_file}", :env => nix_build_env
    end

    #
    # Install Core integrations
    #

    # Create a constraint file after installing all the core dependencies and before any integration
    # This is then used as a constraint file by the integration command to avoid messing with the agent's python environment
    command "#{pip} freeze > #{install_dir}/#{final_constraints_file}"

    # Go through every integration package in `integrations-core`, build and install
    Dir.glob("#{project_dir}/*").each do |check_dir|
      check = check_dir.split('/').last

      # do not install blacklisted integrations
      next if !File.directory?("#{check_dir}") || blacklist_folders.include?(check)

      # If there is no manifest file, then we should assume the folder does not
      # contain a working check and move onto the next
      manifest_file_path = "#{check_dir}/manifest.json"

      # If there is no manifest file, then we should assume the folder does not
      # contain a working check and move onto the next
      File.exist?(manifest_file_path) || next

      manifest = JSON.parse(File.read(manifest_file_path))
      manifest['supported_os'].include?(os) || next

      check_conf_dir = "#{conf_dir}/#{check}.d"

      # For each conf file, if it already exists, that means the `datadog-agent` software def
      # wrote it first. In that case, since the agent's confs take precedence, skip the conf

      # Copy the check config to the conf directories
      conf_file_example = "#{check_dir}/datadog_checks/#{check}/data/conf.yaml.example"
      if File.exist? conf_file_example
        mkdir check_conf_dir
        copy conf_file_example, "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/conf.yaml.example"
      end

      # Copy the default config, if it exists
      conf_file_default = "#{check_dir}/datadog_checks/#{check}/data/conf.yaml.default"
      if File.exist? conf_file_default
        mkdir check_conf_dir
        copy conf_file_default, "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/conf.yaml.default"
      end

      # Copy the metric file, if it exists
      metrics_yaml = "#{check_dir}/datadog_checks/#{check}/data/metrics.yaml"
      if File.exist? metrics_yaml
        mkdir check_conf_dir
        copy metrics_yaml, "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/metrics.yaml"
      end

      # We don't have auto_conf on windows yet
      auto_conf_yaml = "#{check_dir}/datadog_checks/#{check}/data/auto_conf.yaml"
      if File.exist? auto_conf_yaml
        mkdir check_conf_dir
        copy auto_conf_yaml, "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/auto_conf.yaml"
      end

      # Copy SNMP profiles
      profiles = "#{check_dir}/datadog_checks/#{check}/data/profiles"
      if File.exist? profiles
        copy profiles, "#{check_conf_dir}/"
      end

      File.file?("#{check_dir}/setup.py") || next
      if windows?
        command "#{python} -m pip install --no-deps #{windows_safe_path(project_dir)}\\#{check}"
      else
        command "#{pip} install --no-deps .", :env => nix_build_env, :cwd => "#{project_dir}/#{check}"
      end
    end

    # Patch applies to only one file: set it explicitly as a target, no need for -p
    if windows?
      patch :source => "jpype_0_7.patch", :target => "#{python_3_embedded}/Lib/site-packages/jaydebeapi/__init__.py"
    else
      patch :source => "jpype_0_7.patch", :target => "#{install_dir}/embedded/lib/python3.8/site-packages/jaydebeapi/__init__.py"
    end

  end

  # Run pip check to make sure the agent's python environment is clean, all the dependencies are compatible
  if windows?
    command "#{python} -m pip check"
  else
    command "#{pip} check"
  end

  # Ship `requirements-agent-release.txt` file containing the versions of every check shipped with the agent
  # Used by the `datadog-agent integration` command to prevent downgrading a check to a version
  # older than the one shipped in the agent
  copy "#{project_dir}/requirements-agent-release.txt", "#{install_dir}/"
end
