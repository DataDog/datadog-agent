name "setuptools2"
default_version "66.1.1"

skip_transitive_dependency_licensing true

dependency "python2"

relative_path "setuptools-#{version}"

source :url => "https://github.com/pypa/setuptools/archive/v#{version}.tar.gz",
       :sha256 => "081209b1c080b30ca78013dde35600a3070508fcce745a939498f4d76e05b6a6",
       :extract => :seven_zip

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  if ohai["platform"] == "windows"
    python_bin = "#{windows_safe_path(python_2_embedded)}\\python.exe"
    python_prefix = "#{windows_safe_path(python_2_embedded)}"
  else
    python_bin = "#{install_dir}/embedded/bin/python2"
    python_prefix = "#{install_dir}/embedded"
  end

  command "#{python_bin} bootstrap.py"
  command "#{python_bin} setup.py install --prefix=#{python_prefix}"
end
