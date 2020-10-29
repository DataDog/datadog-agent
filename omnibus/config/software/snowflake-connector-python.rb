name "snowflake-connector-python"

default_version "2.1.3"
relative_path "snowflake-connector-python-#{version}"

dependency "cython"

source :url => "https://github.com/snowflakedb/snowflake-connector-python/archive/v#{version}.tar.gz",
       :sha256 => "855ffb93a09c3cd994dab8af7c87a46038bbba103928c5948a0edcd2500f4e1a",
       :extract => :seven_zip

build do
    ship_license "https://raw.githubusercontent.com/snowflakedb/snowflake-connector-python/v#{version}/LICENSE.txt"
    patch :source => "snowflake-connector-python-cryptography.patch", :target => "setup.py"
end