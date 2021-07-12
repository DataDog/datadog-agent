name "aerospike-py2"
default_version "3.10.0"

dependency "aerospike-client"
dependency "pip2"

build do
  env = {
    "DOWNLOAD_C_CLIENT" => "0",
    "AEROSPIKE_C_HOME" => "#{Omnibus::Config.source_dir()}/aerospike-client",
  }

  command "#{install_dir}/embedded/bin/pip2 install --no-binary :all: aerospike==#{version}", :env => env
end
