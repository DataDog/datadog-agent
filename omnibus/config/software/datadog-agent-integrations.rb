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
  if linux?
    conf_directory = "/etc/dd-agent/conf.d"
  elsif osx?
    conf_directory = "#{install_dir}/etc"
  elsif windows?
    conf_directory = "../../extra_package_files/EXAMPLECONFSLOCATION"
  end

  # Copy the checks and generate the global requirements file
  command 'gem install bundle'
  command 'bundle install'
  command "rake copy_checks conf_dir=#{conf_directory} checks_dir=#{checks_dir} merge_requirements_to=."
  # Enqueue "core" dependencies that are not listed in the checks requirements
  command "echo requests==2.11.1 >> check_requirements.txt"

  # FIXME: when the package is renamed `datadog-agent` and replaces the agent5 pkg,
  # ship all the '/etc/dd-agent/conf.d/*' files
  delete '/etc/dd-agent/conf.d/*'

  # Install all the requirements
  if windows?
    pip_args = "install  -r check_requirements.txt"
  
    command "#{windows_safe_path(install_dir)}\\embedded\\scripts\\pip.exe #{pip_args}"
  else
    build_env = {
      "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
      "PATH" => "/#{install_dir}/embedded/bin:#{ENV['PATH']}",
    }
    pip "install -r check_requirements.txt", :env => build_env
  end

  copy '/check_requirements.txt', "#{install_dir}/agent/"
end
