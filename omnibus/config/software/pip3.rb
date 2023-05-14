name "pip3"

# The version of pip used must be at least equal to the one bundled with the Python version we use
# Python 3.8.16 bundles pip 22.0.4
default_version "22.3.1"

skip_transitive_dependency_licensing true

dependency "python3"

source :url => "https://github.com/pypa/pip/archive/#{version}.tar.gz",
       :sha256 => "8d9f7cd8ad0d6f0c70e71704fd3f0f6538d70930454f1f21bbc2f8e94f6964ee",
       :extract => :seven_zip

relative_path "pip-#{version}"

build do
  license "MIT"
  license_file "https://raw.githubusercontent.com/pypa/pip/main/LICENSE.txt"

  if ohai["platform"] == "windows"
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  else
    python = "#{install_dir}/embedded/bin/python3"
  end

  command "#{python} -m pip install ."

  if ohai["platform"] != "windows"
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python3.*/site-packages/pip-*-py3.*.egg/pip/_vendor/distlib/*.exe"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python3.*/site-packages/pip/_vendor/distlib/*.exe"))
    end
  end
end
