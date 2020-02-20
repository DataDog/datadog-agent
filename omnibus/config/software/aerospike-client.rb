name "aerospike-client"

# This needs to be kept in sync with whatever the Python library was built with.
# For example, version 3.10.0 was built with version 4.6.10 of the C library, see:
# https://github.com/aerospike/aerospike-client-python/blob/3.10.0/setup.py#L32-L33
default_version "4.6.10"

source git: "git://github.com/aerospike/aerospike-client-c.git",
       submodules: true

build do
  # The binary wheels on PyPI are not yet compatible with OpenSSL 1.1.0+, see:
  # https://github.com/aerospike/aerospike-client-python/issues/214#issuecomment-385451007
  # https://github.com/aerospike/aerospike-client-python/issues/227#issuecomment-423220411

  env = {
    "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "EXT_CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
  }

  command "make clean", :env => env
  # make fails if used with multiple parallel jobs when building the aerospike client
  command "make", :env => env
end
