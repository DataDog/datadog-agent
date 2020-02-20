name "aerospike-client"

# This needs to be kept in sync with whatever the Python library was built with.
# For example, version 3.10.0 was built with version 4.6.10 of the C library, see:
# https://github.com/aerospike/aerospike-client-python/blob/3.10.0/setup.py#L32-L33
default_version "4.6.10"

build do
  # The binary wheels on PyPI are not yet compatible with OpenSSL 1.1.0+, see:
  # https://github.com/aerospike/aerospike-client-python/issues/214#issuecomment-385451007
  # https://github.com/aerospike/aerospike-client-python/issues/227#issuecomment-423220411
  command "git clone https://github.com/aerospike/aerospike-client-c.git #{install_dir}/embedded/lib/aerospike"
  command "cd #{install_dir}/embedded/lib/aerospike && git checkout #{version}"
  command "cd #{install_dir}/embedded/lib/aerospike && git submodule update --init"

  env = {
    "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "EXT_CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
  }

  command "cd #{install_dir}/embedded/lib/aerospike && make clean", :env => env
  command "cd #{install_dir}/embedded/lib/aerospike && make", :env => env
end
