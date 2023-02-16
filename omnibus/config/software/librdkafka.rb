name "librdkafka"
default_version "2.0.2"

source :url => "https://github.com/confluentinc/librdkafka/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "f321bcb1e015a34114c83cf1aa7b99ee260236aab096b85c003170c90a47ca9d",
       :extract => :seven_zip

relative_path "librdkafka-#{version}"

build do
  license "BSD-style"
  license_file "https://raw.githubusercontent.com/confluentinc/librdkafka/master/LICENSE"

  command "./configure --prefix=#{install_dir}/embedded --install-deps --source-deps-only"
  command "make"
  command "make install", :env => { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }
end
