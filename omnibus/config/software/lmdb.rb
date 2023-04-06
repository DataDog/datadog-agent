name "lmdb"
default_version "0.9.22"

source :url => "https://github.com/LMDB/lmdb/archive/LMDB_#{version}.tar.gz",
       :md5 => "64c6132f481281b7b2ad746ecbfb8423",
       :extract => :seven_zip

relative_path "lmdb-#{version}"

build do
    license ""
    license_file ""
    env = {
        "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
        "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
        "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
    }

    command "make", :env => env
    command "sed -i 's| liblmdb.a||' Makefile" :env => env
    command "make install", :env => env

end
  