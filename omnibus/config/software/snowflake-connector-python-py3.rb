name "snowflake-connector-python-py3"

dependency "pip3"

dependency "snowflake-connector-python"

relative_path "snowflake-connector-python-#{version}"

build do
  command "#{install_dir}/embedded/bin/pip3 install ."
end
