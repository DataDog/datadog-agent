# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

require "./lib/ostools.rb"

name 'datadog-agent-integrations'

dependency 'pip'

relative_path 'integrations-core'
whitelist_file "embedded/lib/python2.7"

source git: 'https://github.com/DataDog/integrations-core.git'
default_version 'master'

build do
  # The checks
  checks_dir = "#{install_dir}/agent/checks.d"
  mkdir checks_dir

  # The confs
  conf_dir = "#{install_dir}/etc/datadog-agent/conf.d"
  mkdir conf_dir
  mkdir "#{conf_dir}/auto_conf"

  # TODO
  # if windows?
  #   conf_directory = "../../extra_package_files/EXAMPLECONFSLOCATION"
  # end

  # Copy the checks and generate the global requirements file
  block do
    all_reqs_file = File.open("#{project_dir}/check_requirements.txt", 'w+')

    Dir.glob("#{project_dir}/*").each do |check_dir|
      # Check the manifest to be sure that this check is enabled on this system
      # or skip this iteration
      manifest_file_path = "#{check_dir}/manifest.json"

      # If there is no manifest file, then we should assume the folder does not
      # contain a working check and move onto the next
      File.exist?(manifest_file_path) || next

      manifest = JSON.parse(File.read(manifest_file_path))
      manifest['supported_os'].include?(os) || next

      check = check_dir.split('/').last

      # Copy the checks over
      if File.exist? "#{check_dir}/check.py"
        copy "#{check_dir}/check.py", "#{checks_dir}/#{check}.py"
      end

      # Copy the check config to the conf directories
      if File.exist? "#{check_dir}/conf.yaml.example"
        copy "#{check_dir}/conf.yaml.example", "#{conf_dir}/#{check}.yaml.example"
      end

      # Copy the default config, if it exists
      if File.exist? "#{check_dir}/conf.yaml.default"
        copy "#{check_dir}/conf.yaml.default", "#{conf_dir}/#{check}.yaml.default"
      end

      # We don't have auto_conf on windows yet
      if os != 'windows'
        if File.exist? "#{check_dir}/auto_conf.yaml"
          copy "#{check_dir}/auto_conf.yaml", "#{conf_dir}/auto_conf/#{check}.yaml"
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

  move "#{project_dir}/check_requirements.txt", "#{install_dir}/agent/"
end
