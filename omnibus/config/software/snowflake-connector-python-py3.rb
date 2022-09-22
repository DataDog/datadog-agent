name "snowflake-connector-python-py3"
default_version "2.7.3"

dependency "pip3"

source :url => "https://github.com/snowflakedb/snowflake-connector-python/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "b5ca1e9957321b5c5422c2333d7d155e2412e63ea583065d56d3371305ef8116",
       :extract => :seven_zip

relative_path "snowflake-connector-python-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"

  # This introduces a pyarrow dependency that is not needed for the agent and fails to build on SUSE
  delete "pyproject.toml"

  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
  end

  command "#{pip} install ."
end
