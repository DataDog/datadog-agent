name "pylint2"
# Ship 1.x as 2.x only supports python 3
default_version "1.9.5"

dependency "pip2"

build do
  # pylint is only called in a subprocess by the Agent, so the Agent doesn't have to be GPL as well
  license "GPL-2.0"

  # aliases for the pips
  if windows_target?
    pip2 = "#{windows_safe_path(python_2_embedded)}\\Scripts\\pip.exe"
    python2 = "#{windows_safe_path(python_2_embedded)}\\python.exe"
  else
    pip2 = "#{install_dir}/embedded/bin/pip2"
    python2 = "#{install_dir}/embedded/bin/python2"
  end

  # If a python_mirror was set, it's passed through a pip config file so that we're not leaking the API key in the CI Output
  # Else the pip config file so pip will act casually
  pip_config_file = ENV['PIP_CONFIG_FILE']
  build_env = {
    "PIP_CONFIG_FILE" => "#{pip_config_file}"
  }

  # pin 2 dependencies of pylint:
  # - configparser: later versions (up to v3.7.1) are broken
  # - lazy-object-proxy 1.7.0 broken on python 2 https://github.com/ionelmc/python-lazy-object-proxy/issues/61
  if windows_target?
    command "#{python2} -m pip install configparser==3.5.0 lazy-object-proxy==1.6.0", :env => build_env
    command "#{python2} -m pip install pylint==#{version}", :env => build_env
  else
    command "#{pip2} install configparser==3.5.0 lazy-object-proxy==1.6.0", :env => build_env
    command "#{pip2} install pylint==#{version}", :env => build_env
  end
end
