name "pip3"

default_version "19.0.3"

dependency "setuptools3"

source :url => "https://github.com/pypa/pip/archive/#{version}.tar.gz",
       :sha256 => "afe5d018b19a8ef00996d6bc3629e6df401efd295c99b38cc4872e07568482ff",
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
