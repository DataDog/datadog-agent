# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

require './lib/ostools.rb'

name 'datadog-agent-integrations'

dependency 'pip'
dependency 'datadog-agent'

relative_path 'integrations-core'
whitelist_file "embedded/lib/python2.7"

source git: 'https://github.com/DataDog/integrations-core.git'

integrations_core_version = ENV['INTEGRATIONS_CORE_VERSION']
if integrations_core_version.nil? || integrations_core_version.empty?
  integrations_core_version = 'master'
end
default_version integrations_core_version


blacklist = [
  'agent_metrics',
  'docker_daemon',
  'kubernetes',
  'ntp',  # provided as a go check by the core agent
  'vsphere',
]

build do
  # The checks
  checks_dir = "#{install_dir}/checks.d"
  mkdir checks_dir

  # The confs
  if osx?
    conf_dir = "#{install_dir}/etc/conf.d"
  else
    conf_dir = "#{install_dir}/etc/datadog-agent/conf.d"
  end
  mkdir conf_dir

  # Copy the checks and generate the global requirements file
  block do
    all_reqs_file = File.open("#{project_dir}/check_requirements.txt", 'w+')

    Dir.glob("#{project_dir}/*").each do |check_dir|
      check = check_dir.split('/').last

      next if blacklist.include? check
      # Check the manifest to be sure that this check is enabled on this system
      # or skip this iteration
      manifest_file_path = "#{check_dir}/manifest.json"

      # If there is no manifest file, then we should assume the folder does not
      # contain a working check and move onto the next
      File.exist?(manifest_file_path) || next

      manifest = JSON.parse(File.read(manifest_file_path))
      manifest['supported_os'].include?(os) || next

      # Copy the checks over
      if File.exist? "#{check_dir}/check.py"
        copy "#{check_dir}/check.py", "#{checks_dir}/#{check}.py"
      end

      check_conf_dir = "#{conf_dir}/#{check}.d"

      mkdir check_conf_dir unless File.exist? check_conf_dir

      # For each conf file, if it already exists, that means the `datadog-agent` software def
      # wrote it first. In that case, since the agent's confs take precedence, skip the conf

      # Copy the check config to the conf directories
      if File.exist? "#{check_dir}/conf.yaml.example"
        copy "#{check_dir}/conf.yaml.example", "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/conf.yaml.example"
      end

      # Copy the default config, if it exists
      if File.exist? "#{check_dir}/conf.yaml.default"
        mkdir check_conf_dir
        copy "#{check_dir}/conf.yaml.default", "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/conf.yaml.default"
      end

      # Copy the metric file, if it exists
      if File.exist? "#{check_dir}/metrics.yaml"
        mkdir check_conf_dir
        copy "#{check_dir}/metrics.yaml", "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/metrics.yaml"
      end

      # We don't have auto_conf on windows yet
      if os != 'windows'
        if File.exist? "#{check_dir}/auto_conf.yaml"
          mkdir check_conf_dir
          copy "#{check_dir}/auto_conf.yaml", "#{check_conf_dir}/" unless File.exist? "#{check_conf_dir}/auto_conf.yaml"
        end
      end

      next unless File.exist?("#{check_dir}/requirements.txt") && !manifest['use_omnibus_reqs']

      reqs = File.open("#{check_dir}/requirements.txt", 'r').read
      reqs.each_line do |line|
        all_reqs_file.puts line if line[0] != '#'
      end
    end

    # Manually add "core" dependencies that are not listed in the checks requirements
    all_reqs_file.puts "requests==2.11.1"
    all_reqs_file.puts "pympler==0.5"

    all_reqs_file.close
  end

  # Install all the requirements
  if windows?
    pip_args = "install  -r #{project_dir}/check_requirements.txt"
    command "#{windows_safe_path(install_dir)}\\embedded\\scripts\\pip.exe #{pip_args}"
  else
    build_env = {
      "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
      "PATH" => "#{install_dir}/embedded/bin:#{ENV['PATH']}",
    }
    pip "install -r #{project_dir}/check_requirements.txt", :env => build_env
  end
end
