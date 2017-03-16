name 'datadog-agent-integrations'

dependency 'pip'
dependency 'datadog-agent'

relative_path 'integrations-core'

source git: 'https://github.com/DataDog/integrations-core.git'
default_version 'massi/rakefile'

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

  # Copy the checks and generate the requriments file
  rake "copy_checks conf_dir=#{conf_directory} checks_dir=#{checks_dir}"

  # Install all the requirements
  pip_args = "install --install-option=\"--install-scripts=#{windows_safe_path(install_dir)}/bin\" -r check_requirements.txt"
  if windows?
    command "#{windows_safe_path(install_dir)}\\embedded\\scripts\\pip.exe #{pip_args}"
  else
    build_env = {
      "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
      "PATH" => "/#{install_dir}/embedded/bin:#{ENV['PATH']}",
    }
    command "pip #{pip_args}", :env => build_env
  end

  copy '/check_requirements.txt', "#{install_dir}/agent/"
end