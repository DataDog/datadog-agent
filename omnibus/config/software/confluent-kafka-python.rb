# https://github.com/confluentinc/confluent-kafka-python/blob/master/INSTALL.md#install-from-source

name "confluent-kafka-python"
default_version "2.0.2"

dependency "pip3"
dependency "librdkafka"

source :url => "https://github.com/confluentinc/confluent-kafka-python/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "137c82d23e931e03a2803569d51bb025a7b9a364818b08d1664800abaaa5cc63",
       :extract => :seven_zip

relative_path "confluent-kafka-python-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"



  if windows?
    # See `librdkafka.rb`
    librd_dir = "c:\\librdkafka-redist"
    build_env = {
       "INCLUDE" => "#{librd_dir}\\librdkafka.redist.#{version}\\build\\native\\include",
       "LIB" => "#{librd_dir}\\librdkafka.redist.#{version}\\build\\native\\lib\\win\\x64\\win-x64-Release\\v142"
    }

    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    build_env = {
      "CFLAGS" => "-I#{install_dir}/embedded/include -std=c99"
    }
    pip = "#{install_dir}/embedded/bin/pip3"
  end

  command "#{pip} install --no-binary confluent-kafka .", :env => build_env

  if windows?
    # https://github.com/confluentinc/confluent-kafka-python/blob/master/tools/windows-build.bat
    # https://github.com/confluentinc/confluent-kafka-python/blob/master/tools/windows-copy-librdkafka.bat
    # https://github.com/confluentinc/confluent-kafka-python/blob/master/tools/windows-install-librdkafka.bat

    ## seem to need to copy this by hand
    ## why is this not handled by pip install?
    librd_bindir = "#{librd_dir}\\librdkafka.redist.#{version}\\runtimes\\win-x64\\native"
    librd_target = "#{windows_safe_path(python_3_embedded)}\\Lib\\site-packages\\confluent_kafka"

    needed_dlls = ["libcrypto-3-x64.dll",
      "libcurl.dll",
      "librdkafka.dll",
      "librdkafkacpp.dll",
      "libssl-3-x64.dll",
      ## do not for any reason copy  the base C runtime DLLS that come with the librdkafka
      ## package.  They would likely mess up the entire rest of the windows agent python distro
      ##"msvcp140.dll",
      ##"vcruntime140.dll",
      "zlib1.dll",
      "zstd.dll"]

    needed_dlls.each do |dll|
      copy "#{librd_bindir}\\#{dll}", "#{librd_target}"
    end
  end
end
