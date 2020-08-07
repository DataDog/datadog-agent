name "pip3"

default_version "20.1.1"

dependency "setuptools3"

source :url => "https://github.com/pypa/pip/archive/#{version}.tar.gz",
       :sha256 => "fa20f7632bab63162d281e555e1d40dced21f22b2578709454f9015f279a0144",
       :extract => :seven_zip

relative_path "pip-#{version}"

build do
  ship_license "https://raw.githubusercontent.com/pypa/pip/develop/LICENSE.txt"

  if ohai["platform"] == "windows"
    python_bin = "#{windows_safe_path(python_3_embedded)}\\python.exe"
    python_prefix = "#{windows_safe_path(python_3_embedded)}"
  else
    python_bin = "#{install_dir}/embedded/bin/python3"
    python_prefix = "#{install_dir}/embedded"
  end

  command "#{python_bin} setup.py install --prefix=#{python_prefix}"

  if ohai["platform"] != "windows"
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python3.*/site-packages/pip-*-py3.*.egg/pip/_vendor/distlib/*.exe"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python3.*/site-packages/pip/_vendor/distlib/*.exe"))
    end
  end
end
