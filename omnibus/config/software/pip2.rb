name "pip2"

default_version "20.1.1"

dependency "setuptools2"

source :url => "https://github.com/pypa/pip/archive/#{version}.tar.gz",
       :sha256 => "fa20f7632bab63162d281e555e1d40dced21f22b2578709454f9015f279a0144",
       :extract => :seven_zip

relative_path "pip-#{version}"

build do
  ship_license "https://raw.githubusercontent.com/pypa/pip/develop/LICENSE.txt"

  patch :source => "remove-python27-deprecation-warning.patch", :target => "src/pip/_internal/cli/base_command.py"

  if ohai["platform"] == "windows"
    python_bin = "#{windows_safe_path(python_2_embedded)}\\python.exe"
    python_prefix = "#{windows_safe_path(python_2_embedded)}"
  else
    python_bin = "#{install_dir}/embedded/bin/python2"
    python_prefix = "#{install_dir}/embedded"
  end

  command "#{python_bin} setup.py install --prefix=#{python_prefix}"

  if ohai["platform"] != "windows"
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python2.7/site-packages/pip-*-py2.7.egg/pip/_vendor/distlib/*.exe"))
    end
  end
end
