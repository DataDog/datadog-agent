name "e2fsprogs"
default_version "1.47.0"

source :url => "https://mirrors.edge.kernel.org/pub/linux/kernel/people/tytso/e2fsprogs/v#{version}/e2fsprogs-#{version}.tar.gz",
       :sha256 => "0b4fe723d779b0927fb83c9ae709bc7b40f66d7df36433bef143e41c54257084",
       :extract => :seven_zip

relative_path "e2fsprogs-#{version}"

build do

  env = {
    "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
  }

  command "./configure --prefix=#{install_dir}/embedded", :env => env
  command "make", :env => env
  command "make install", :env => env

end
