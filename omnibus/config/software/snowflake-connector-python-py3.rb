name "snowflake-connector-python-py3"

dependency "pip3"

dependency "snowflake-connector-python"

build do
  command "cd #{Omnibus::Config.source_dir()}/snowflake-connector-python/snowflake-connector-python-2.1.3 && #{install_dir}/embedded/bin/pip3 install ."
end