# https://github.com/confluentinc/librdkafka#build-from-source
# https://github.com/Homebrew/homebrew-core/blob/35f8763a90eab4203deb3a6ee2503cf0ddfd8c84/Formula/librdkafka.rb#L32
# https://github.com/confluentinc/confluent-kafka-python/blob/master/tools/windows-install-librdkafka.bat

name "librdkafka"
default_version "2.3.0"

dependency "cyrus-sasl"
dependency "curl"

source :url => "https://github.com/confluentinc/librdkafka/archive/refs/tags/v#{version}.tar.gz",
        :sha256 => "2d49c35c77eeb3d42fa61c43757fcbb6a206daa560247154e60642bcdcc14d12",
        :extract => :seven_zip

relative_path "librdkafka-#{version}"

build do

  license "BSD-style"
  license_file "https://raw.githubusercontent.com/confluentinc/librdkafka/master/LICENSE"

  env = with_standard_compiler_flags(with_embedded_path)
  configure_options = [
    "--enable-sasl",
    "--enable-curl"
  ]
  configure(*configure_options, :env => env)
  command "make -j #{workers}", :env => env
  command "make install", :env => env

  delete "#{install_dir}/embedded/lib/librdkafka.a"
  delete "#{install_dir}/embedded/lib/librdkafka++.a"
  delete "#{install_dir}/embedded/lib/librdkafka-static.a"

end
