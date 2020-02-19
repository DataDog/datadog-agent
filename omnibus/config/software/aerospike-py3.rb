name "aerospike-py3"
default_version "3.10.0"

dependency "aerospike-client"
dependency "pip3"

build do
  env = {
    "DOWNLOAD_C_CLIENT" => "0",
    "AEROSPIKE_C_HOME" => "#{install_dir}/embedded/lib/aerospike",
  }

  command "#{install_dir}/embedded/bin/pip3 install --no-binary :all: aerospike==#{version}", :env => env
end
