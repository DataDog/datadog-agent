# https://github.com/confluentinc/confluent-kafka-python/blob/master/INSTALL.md#install-from-source

name "confluent-kafka-python"
default_version "2.1.1"

dependency "pip3"

source :url => "https://github.com/confluentinc/confluent-kafka-python/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "b1abd74866f4dab5042b64262be3d330a67ab7a535d1f7c31d5ecc4834532adf",
       :extract => :seven_zip

relative_path "confluent-kafka-python-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"

  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
    command "#{pip} install confluent-kafka"
  else
    dependency "librdkafka"

    build_env = {
      "CFLAGS" => "-I#{install_dir}/embedded/include -std=c99"
    }
    pip = "#{install_dir}/embedded/bin/pip3"
    command "#{pip} install --no-binary confluent-kafka .", :env => build_env
  end

  

end
