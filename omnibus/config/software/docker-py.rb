name "docker-py"
default_version "1.10.6"

dependency "python"
dependency "pip"

build do
  ship_license "Apachev2"
  command "#{install_dir}/embedded/scripts/pip install #{name}==#{version}"
end