name "datadog-a7-py3"
default_version "0.0.7"

dependency "pip3"

build do
  ship_license "https://raw.githubusercontent.com/DataDog/datadog-checks-shared/master/LICENSE"

  # aliases for the pips
  if windows?
    pip3 = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
    python3 = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  else
    pip3 = "#{install_dir}/embedded/bin/pip3"
    python3 = "#{install_dir}/embedded/bin/python3"
  end

  if windows?
    # this pins a dependency of pylint->datadog-a7, later versions (up to v3.7.1) are broken.
    command "#{python3} -m pip install configparser==3.5.0"
    command "#{python3} -m pip install datadog-a7==#{version}"
  else
    command "#{pip3} install configparser==3.5.0"
    command "#{pip3} install datadog-a7==#{version}"
  end
end
