name "pip3"

# The version of pip used must be at least equal to the one bundled with the Python version we use
# Python 3.9.17 bundles pip 23.0.1
default_version "23.0.1"

skip_transitive_dependency_licensing true

dependency "python3"

source :url => "https://github.com/pypa/pip/archive/#{version}.tar.gz",
       :sha256 => "8544443b6810cf1e41306f44218449524d579f4f801b6b16e46f7cabe64de155",
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
