name "snowflake-connector-python-py3"

dependency "pip3"

default_version "2.6.0"


source :url => "https://github.com/snowflakedb/snowflake-connector-python/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "bb7af6933bdd6b8b105dac304de66fdb03e0b17378d588b5be6f1026b6ce3674",
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
  command "#{pip} install ."
end