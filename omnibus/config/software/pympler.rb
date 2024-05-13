# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Even though this is a dependency that we install with `pip`, it makes sense to keep it
# separate from the integrations-related definitions since it's not defined anywhere as
# a dependency for integrations.
name 'pympler'
default_version "0.7"

if with_python_runtime? "3"
  dependency 'pip3'
  dependency 'setuptools3'
end

if with_python_runtime? "2"
  dependency 'pip2'
end

pympler_requirement = "pympler==#{default_version}"

build do
  if with_python_runtime? "3"
    if windows_target?
      python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
    else
      python = "#{install_dir}/embedded/bin/python3"
    end
    command "#{python} -m pip install #{pympler_requirement}"
  end

  if with_python_runtime? "2"
    if windows_target?
      python = "#{windows_safe_path(python_2_embedded)}\\python.exe"
    else
      python = "#{install_dir}/embedded/bin/python2"
    end
    command "#{python} -m pip install #{pympler_requirement}"
  end

end
