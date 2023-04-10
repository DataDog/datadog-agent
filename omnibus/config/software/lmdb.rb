name "lmdb"
default_version "0.9.29"

source :url => "https://github.com/LMDB/lmdb/archive/LMDB_#{version}.tar.gz",
       :sha256 => "22054926b426c66d8f2bc22071365df6e35f3aacf19ad943bc6167d4cae3bebb",
       :extract => :seven_zip

relative_path "lmdb-LMDB_#{version}/libraries/liblmdb"

build do
    license "OpenLDAP Public License"
    license_file "https://raw.githubusercontent.com/LMDB/lmdb/LMDB_#{version}/libraries/liblmdb/COPYRIGHT"
    env = {
        "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
        "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
        "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
    }

    # https://www.linuxfromscratch.org/blfs/view/8.3/server/lmdb.html
    command "make", :env => env
    command "sed -i 's| liblmdb.a||' Makefile", :env => env
    command "make install", :env => env

end
  