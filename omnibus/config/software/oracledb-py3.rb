name "oracledb-py3"
default_version "1.3.2"

dependency "pip3"

source :url => "https://github.com/oracle/python-oracledb/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "0a8a6bd2aacf7db0f26b0cb9991316f3e02f4bf671c67ca38b0baa9fd6fbb21f",
       :extract => :seven_zip

relative_path "python-oracledb-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"

  command "sed -i 's/cython/cython<3.0.0/g' pyproject.toml"

  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
  end

  command "#{pip} install ."
end
