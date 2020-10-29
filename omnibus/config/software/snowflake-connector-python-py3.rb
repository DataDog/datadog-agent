name "snowflake-connector-python-py3"

dependency "pip3"

dependency "snowflake-connector-python"

build do
  ship_license "https://raw.githubusercontent.com/snowflakedb/snowflake-connector-python/v#{version}/LICENSE.txt"
  patch :source => "snowflake-connector-python-cryptography.patch", :target => "setup.py"
  command "#{install_dir}/embedded/bin/pip3 install ."
end
