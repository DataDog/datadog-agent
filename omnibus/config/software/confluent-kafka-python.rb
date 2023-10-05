# https://github.com/confluentinc/confluent-kafka-python/blob/master/INSTALL.md#install-from-source

name "confluent-kafka-python"
default_version "2.2.0"

dependency "pip3"

source :url => "https://github.com/confluentinc/confluent-kafka-python/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "ee099702bd5fccd3ce4916658fed4c7ef28cb22e111defb843d27633100ff065",
       :extract => :seven_zip

relative_path "confluent-kafka-python-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"

  if windows_target?
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
