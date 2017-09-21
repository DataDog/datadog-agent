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

  # TODO
  # if windows?
  #   conf_directory = "../../extra_package_files/EXAMPLECONFSLOCATION"
  # end

  # Copy the checks and generate the global requirements file
  block do
    all_reqs_file = File.open("#{project_dir}/check_requirements.txt", 'w+')

    Dir.glob('*/').each do |check|
      check.slice! '/'

      # Check the manifest to be sure that this check is enabled on this system
      # or skip this iteration
      manifest_file_path = "#{check}/manifest.json"

      # If there is no manifest file, then we should assume the folder does not
      # contain a working check and move onto the next
      File.exist?(manifest_file_path) || next

      manifest = JSON.parse(File.read(manifest_file_path))
      manifest['supported_os'].include?(os) || next

      # Copy the checks over
      if File.exist? "#{check}/check.py"
        copy "#{check}/check.py", "#{checks_dir}/#{check}.py"
      end

      # Copy the check config to the conf directories
      if File.exist? "#{check}/conf.yaml.example"
        copy "#{check}/conf.yaml.example", "#{conf_dir}/#{check}.yaml.example"
      end

      # Copy the default config, if it exists
      if File.exist? "#{check}/conf.yaml.default"
        copy "#{check}/conf.yaml.default", "#{conf_dir}/#{check}.yaml.default"
      end

      # We don't have auto_conf on windows yet
      if os != 'windows'
        if File.exist? "#{check}/auto_conf.yaml"
          copy "#{check}/auto_conf.yaml", "#{conf_dir}/auto_conf/#{check}.yaml"
        end
      end

      next unless File.exist?("#{check}/requirements.txt") && !manifest['use_omnibus_reqs']

      reqs = File.open("#{check}/requirements.txt", 'r').read
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
    command "pip install -r #{project_dir}/check_requirements.txt", :env => build_env
  end

  move "#{project_dir}/check_requirements.txt", "#{install_dir}/agent/"
end
