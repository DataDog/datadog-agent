name "snowflake-connector-python-py2"

dependency "pip2"

dependency "snowflake-connector-python"

build do
  command "#{install_dir}/embedded/bin/pip2 install ."
end
