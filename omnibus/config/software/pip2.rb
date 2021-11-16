name "pip2"
default_version "20.3.3"

dependency "setuptools2"


source :url => "https://github.com/pypa/pip/archive/#{version}.tar.gz",
       :sha256 => "016f8d509871b72fb05da911db513c11059d8a99f4591dda3050a3cf83a29a79",
       :extract => :seven_zip

relative_path "pip-#{version}"

build do
  license "MIT"
  license_file "https://raw.githubusercontent.com/pypa/pip/main/LICENSE.txt"

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
