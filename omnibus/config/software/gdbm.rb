name "gdbm"
default_version "1.23"

source :url => "http://ftp.gnu.org/gnu/gdbm/gdbm-#{version}.tar.gz",
       :sha256 => "74b1081d21fff13ae4bd7c16e5d6e504a4c26f7cde1dca0d963a484174bbcacd"
       :extract => :seven_zip

relative_path "gdbm-#{version}"

build do
  env = {
    "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
  }

  configure_command = ["./configure",
                       "--enable-libgdbm-compat",
                       "--prefix=#{install_dir}/embedded"]

  command configure_command.join(" "), env: env
  command "make -j #{workers}", env: env
  command "make install", env: env
end
