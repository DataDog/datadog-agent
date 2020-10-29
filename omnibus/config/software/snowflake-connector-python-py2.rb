name "snowflake-connector-python-py2"

dependency "pip2"

dependency "snowflake-connector-python"

version "2.1.3"

build do
  command "#{install_dir}/embedded/bin/pip2 install --no-deps #{Omnibus::Config.source_dir()}/snowflake-connector-python/snowflake-connector-python-#{version}"
end
