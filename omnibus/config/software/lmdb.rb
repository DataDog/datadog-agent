name "lmdb"
default_version "0.9.29"

source :sha512 => "f75d5751ed97c1b7f982379988719f566efbf8df2d4c8894764f49c2eb926e3918844bc1e7d88e8b278e1c949ad75940f2404816ce345e74cf94d36645143b05",
       :url => "https://src.fedoraproject.org/repo/pkgs/lmdb/LMDB_#{version}.tar.gz/sha512/f75d5751ed97c1b7f982379988719f566efbf8df2d4c8894764f49c2eb926e3918844bc1e7d88e8b278e1c949ad75940f2404816ce345e74cf94d36645143b05/LMDB_#{version}.tar.gz",
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

    configure_command = ["./configure",
                          "--prefix=#{install_dir}/embedded"]  
  
    command configure_command.join(" "), env: env
    command "make", :env => env
    command "make install", :env => env

end
  