# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

require './lib/ostools.rb'
require 'json'

name 'stackstate-agent-integrations'

dependency 'pip'
dependency 'datadog-agent'
dependency 'protobuf-py'

if linux?
  # add nfsiostat script
  dependency 'unixodbc'
  dependency 'nfsiostat'
end

unless windows?
  # need kerberos for hdfs
  dependency 'libkrb5'
end

relative_path 'integrations-core'
whitelist_file "embedded/lib/python2.7"

source git: 'https://github.com/StackVista/stackstate-agent-integrations.git'

PIPTOOLS_VERSION = "2.0.2"
UNINSTALL_PIPTOOLS_DEPS = ['six', 'click', 'first', 'pip-tools']

integrations_core_version = ENV['STACKSTATE_INTEGRATIONS_VERSION']
if integrations_core_version.nil? || integrations_core_version.empty?
  integrations_core_version = 'master'
end
default_version integrations_core_version


blacklist = [
  'stackstate_checks_base',           # namespacing package for wheels (NOT AN INTEGRATION)
  'stackstate_checks_dev',            # Development package, (NOT AN INTEGRATION)
  'stackstate_checks_tests_helper'    # Testing and Development package, (NOT AN INTEGRATION)
]
blacklist_req = Array.new

if suse?
  blacklist.push('aerospike')  # Temporarily blacklist Aerospike until builder supports new dependency
  blacklist_req.push(/^aerospike==/)  # Temporarily blacklist Aerospike until builder supports new dependency
end

core_constraints_file = 'core_constraints.txt'
final_constraints_file = 'final_constraints.txt'
agent_requirements_file = 'agent_requirements.txt'
agent_requirements_in = 'agent_requirements.in'

build do
  # The dir for the confs
  if osx?
    conf_dir = "#{install_dir}/etc/conf.d"
  else
    conf_dir = "#{install_dir}/etc/stackstate-agent/conf.d"
  end
  mkdir conf_dir

  # Install the checks and generate the global requirements file
  block do
    all_reqs_file = File.open("#{project_dir}/check_requirements.txt", 'w+')
    # FIX THIS these dependencies have to be grabbed from somewhere
    all_reqs_file.puts "pympler==0.5 --hash=sha256:7d16c4285f01dcc647f69fb6ed4635788abc7a7cb7caa0065d763f4ce3d21c0f"
    all_reqs_file.puts "wheel==0.30.0 --hash=sha256:e721e53864f084f956f40f96124a74da0631ac13fbbd1ba99e8e2b5e9cafdf64"\
    " --hash=sha256:9515fe0a94e823fd90b08d22de45d7bde57c90edce705b22f5e1ecf7e1b653c8"

    all_reqs_file.close

    nix_build_env = {
      "CFLAGS" => "-I#{install_dir}/embedded/include -I/opt/mqm/inc",
      "CXXFLAGS" => "-I#{install_dir}/embedded/include -I/opt/mqm/inc",
      "LDFLAGS" => "-L#{install_dir}/embedded/lib -L/opt/mqm/lib64 -L/opt/mqm/lib",
      "LD_RUN_PATH" => "#{install_dir}/embedded/lib -L/opt/mqm/lib64 -L/opt/mqm/lib",
      "PATH" => "#{install_dir}/embedded/bin:#{ENV['PATH']}",
    }

    # Install all the requirements
    # Install all the build requirements
    if windows?
      pip_args = "install --require-hashes -r #{project_dir}/check_requirements.txt"
      command "#{windows_safe_path(install_dir)}\\embedded\\scripts\\pip.exe #{pip_args}"
    else
      pip "install --require-hashes -r #{project_dir}/check_requirements.txt", :env => nix_build_env
    end

    # Set frozen requirements (save to var, and to file)
    # HACK: we need to do this like this due to the well known issues with omnibus
    # runtime requirements.
    if windows?
      freeze_mixin = shellout!("#{windows_safe_path(install_dir)}\\embedded\\Scripts\\pip.exe freeze")
      frozen_agent_reqs = freeze_mixin.stdout
    else
      freeze_mixin = shellout!("#{install_dir}/embedded/bin/pip freeze")
      frozen_agent_reqs = freeze_mixin.stdout
    end
    pip "freeze > #{project_dir}/#{core_constraints_file}"

    # Install all the build requirements
    if windows?
      pip_args = "install pip-tools==#{PIPTOOLS_VERSION}"
      command "#{windows_safe_path(install_dir)}\\embedded\\scripts\\pip.exe #{pip_args}"
    else
      pip "install pip-tools==#{PIPTOOLS_VERSION}", :env => nix_build_env
    end

    # Windows pip workaround to support globs
    python_bin = "\"#{windows_safe_path(install_dir)}\\embedded\\python.exe\""
    python_pip_no_deps = "pip install -c #{windows_safe_path(project_dir)}\\#{core_constraints_file} --no-deps #{windows_safe_path(project_dir)}"
    python_pip_req = "pip install -c #{windows_safe_path(project_dir)}\\#{core_constraints_file} --no-deps --require-hashes -r"
    python_pip_uninstall = "pip uninstall -y"

    if windows?
      static_reqs_in_file = "#{windows_safe_path(project_dir)}\\stackstate_checks_base\\stackstate_checks\\base\\data\\#{agent_requirements_in}"
      static_reqs_filtered_file = "#{windows_safe_path(project_dir)}\\#{agent_requirements_in}"
    else
      static_reqs_in_file = "#{project_dir}/stackstate_checks_base/stackstate_checks/base/data/#{agent_requirements_in}"
      static_reqs_filtered_file = "#{project_dir}/#{agent_requirements_in}"
    end

    # Remove any blacklisted requirements from the static-environment req file
    requirements = Array.new
    File.open("#{static_reqs_in_file}", 'r+').readlines().each do |line|
      blacklist_flag = false
      blacklist_req.each do |blacklist_regex|
        re = Regexp.new(blacklist_regex).freeze
        if re.match line
          blacklist_flag = true
        end
      end

      if !blacklist_flag
        requirements.push(line)
      end
    end

    erb source: "static_requirements.txt.erb",
        dest: "#{static_reqs_filtered_file}",
        mode: 0640,
        vars: { requirements: requirements }

    # Install the static environment requirements that the Agent and all checks will use
    if windows?
      command("#{python_bin} -m #{python_pip_no_deps}\\stackstate_checks_base")
      command("#{python_bin} -m piptools compile --generate-hashes --output-file #{windows_safe_path(install_dir)}\\#{agent_requirements_file} #{static_reqs_filtered_file}")
    else
      pip "install -c #{project_dir}/#{core_constraints_file} --no-deps .", :env => nix_build_env, :cwd => "#{project_dir}/stackstate_checks_base"
      command("#{install_dir}/embedded/bin/python -m piptools compile --generate-hashes --output-file #{install_dir}/#{agent_requirements_file} #{static_reqs_filtered_file}")
    end

    # Uninstall the deps that pip-compile installs so we don't include them in the final artifact
    UNINSTALL_PIPTOOLS_DEPS.each do |dep|
      re = Regexp.new("^#{dep}==").freeze
      if not re.match frozen_agent_reqs
        if windows?
          command("#{python_bin} -m #{python_pip_uninstall} #{dep}")
        else
          pip "uninstall -y #{dep}"
        end
      end
    end

    # install static-environment requirements
    if windows?
      command("#{python_bin} -m #{python_pip_req} #{windows_safe_path(install_dir)}\\#{agent_requirements_file}")
    else
      pip "install -c #{project_dir}/#{core_constraints_file} --require-hashes --no-deps -r #{install_dir}/#{agent_requirements_file}", :env => nix_build_env
    end
    # Create a constraint file after installing all the core dependencies and before any integration
    # This is then used as a constraint file by the integration command to avoid messing with the agent's python environment
    pip "freeze > #{install_dir}/#{final_constraints_file}"

    # Ship requirements-agent-release.txt file containing the versions of every check shipped with the agent
    # Used by the `stackstate-agent integration` command to prevent downgrading a check to a version
    # older than the one shipped in the agent
    copy "#{project_dir}/requirements-agent-release.txt", "#{install_dir}/"

    # install integrations
    Dir.glob("#{project_dir}/*").each do |check_dir|
      check = check_dir.split('/').last

      next if !File.directory?("#{check_dir}") || blacklist.include?(check)

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
      conf_file_example = "#{check_dir}/stackstate_checks/#{check}/data/conf.yaml.example"
      if File.exist? conf_file_example
        mkdir check_conf_dir
        copy conf_file_example, "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/conf.yaml.example"
      end

      # Copy the default config, if it exists
      conf_file_default = "#{check_dir}/stackstate_checks/#{check}/data/conf.yaml.default"
      if File.exist? conf_file_default
        mkdir check_conf_dir
        copy conf_file_default, "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/conf.yaml.default"
      end

      # Copy the metric file, if it exists
      metrics_yaml = "#{check_dir}/stackstate_checks/#{check}/data/metrics.yaml"
      if File.exist? metrics_yaml
        mkdir check_conf_dir
        copy metrics_yaml, "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/metrics.yaml"
      end

      # We don't have auto_conf on windows yet
      if os != 'windows'
        auto_conf_yaml = "#{check_dir}/stackstate_checks/#{check}/data/auto_conf.yaml"
        if File.exist? auto_conf_yaml
          mkdir check_conf_dir
          copy auto_conf_yaml, "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/auto_conf.yaml"
        end
      end

      File.file?("#{check_dir}/setup.py") || next
      if windows?
        command("#{python_bin} -m #{python_pip_no_deps}\\#{check}")
      else
        pip "install --no-deps .", :env => nix_build_env, :cwd => "#{project_dir}/#{check}"
      end
    end
  end
end
