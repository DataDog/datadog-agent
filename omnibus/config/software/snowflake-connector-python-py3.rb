name "snowflake-connector-python-py2"

dependency "pip2"

dependency "snowflake-connector-python"

build do
  command "cd #{Omnibus::Config.source_dir()}/snowflake-connector-python/snowflake-connector-python-2.1.3 && #{install_dir}/embedded/bin/pip2 install ."
end