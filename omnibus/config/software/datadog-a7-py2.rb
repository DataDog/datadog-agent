name "datadog-a7-py2"
default_version "0.0.5"

dependency "pip2"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/datadog-checks-shared/master/LICENSE"

  # aliases for the pips
  if windows?
    pip2 = "#{windows_safe_path(python_2_embedded)}\\Scripts\\pip.exe"
    python2 = "#{windows_safe_path(python_2_embedded)}\\python.exe"
  else
    pip2 = "#{install_dir}/embedded/bin/pip2"
    python2 = "#{install_dir}/embedded/bin/python2"
  end

  if windows?
    # this pins a dependency of pylint->datadog-a7, later versions (up to v3.7.1) are broken.
    command "#{python2} -m pip install configparser==3.5.0"
    command "#{python2} -m pip install datadog-a7==#{version} --install-option=\"--install-scripts=#{windows_safe_path(install_dir)}/bin\""
  else
    command "#{pip2} install configparser==3.5.0"
    command "#{pip2} install datadog-a7==#{version}"
  end
end
