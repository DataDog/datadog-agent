name "setuptools"
default_version "28.8.0"

dependency "python"

relative_path "setuptools-#{version}"

source :url => "https://github.com/pypa/setuptools/archive/v#{version}.tar.gz",
       :sha256 => "d3b2c63a5cb6816ace0883bc3f6aca9e7890c61d80ac0d608a183f85825a7cc0"

build do
  python_path = "#{install_dir}/embedded/bin/python"
  if ohai["platform"] == "windows"
    python_path = "#{windows_safe_path(install_dir)}\\embedded\\python.exe"
  end

  ship_license "PSFL"
  command "#{python_path} bootstrap.py"
  command "#{python_path} setup.py install --prefix=#{windows_safe_path(install_dir)}/embedded"
end