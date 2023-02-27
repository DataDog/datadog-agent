name "cyrus-sasl"
default_version "2.1.28"

source :url => "https://github.com/cyrusimap/cyrus-sasl/releases/download/cyrus-sasl-#{version}/cyrus-sasl-#{version}.tar.gz",
       :sha256 => "f321bcb1e015a34114c83cf1aa7b99ee260236aab096b85c003170c90a47ca9d",
       :extract => :seven_zip

relative_path "cyrus-sasl-#{version}"

build do

  env = {
    "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
  }

  command "./configure --prefix=#{install_dir}/embedded", :env => env
  command "make", :env => { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }
  command "make install", :env => { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }

end
