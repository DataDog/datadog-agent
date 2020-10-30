name "snowflake-connector-python-py2"

dependency "pip2"
dependency "cython2"

default_version "2.1.3"

source :url => "https://github.com/snowflakedb/snowflake-connector-python/archive/v#{version}.tar.gz",
       :sha256 => "855ffb93a09c3cd994dab8af7c87a46038bbba103928c5948a0edcd2500f4e1a",
       :extract => :seven_zip

relative_path "snowflake-connector-python-#{version}"

build do
    ship_license "https://raw.githubusercontent.com/snowflakedb/snowflake-connector-python/v#{version}/LICENSE.txt"
    patch :source => "cryptography-dependency.patch", :target => "setup.py"
    command "#{install_dir}/embedded/bin/pip2 install ."
end
