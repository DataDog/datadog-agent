name "snowflake-connector-python-py3"

dependency "pip3"

dependency "snowflake-connector-python"

default_version "2.1.3"
relative_path "snowflake-connector-python-#{version}"

build do
  command "#{install_dir}/embedded/bin/pip3 install ."
end
