name "snowflake-connector-python-py3"

dependency "pip3"

default_version "2.6.0"


source :url => "https://github.com/snowflakedb/snowflake-connector-python/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "31764cb1ee30a575d8b15aa3af1df8795d30838f",
       :extract => :seven_zip

relative_path "snowflake-connector-python-#{version}"

build do
  # This introduces a pyarrow dependency that is not needed for the agent and fails to build on SUSE
  delete "pyproject.toml"

  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
  end


  ship_license "https://raw.githubusercontent.com/snowflakedb/snowflake-connector-python/v#{version}/LICENSE.txt"
  patch :source => "dependencies.patch", :target => "setup.py"
  command "#{pip} install ."
end