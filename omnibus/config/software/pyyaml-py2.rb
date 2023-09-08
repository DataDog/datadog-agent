name "pyyaml-py2"
default_version "5.4.1"

dependency "pip2"

source :url => "https://github.com/yaml/pyyaml/archive/refs/tags/#{version}.tar.gz",
       :sha256 => "75f966559c5f262dfc44da0f958cc2aa18953ae5021f2c3657b415c5a370045f",
       :extract => :seven_zip

relative_path "pyyaml-#{version}"

build do
  license "MIT"
  license_file "./LICENSE.txt"

  command "sed -i 's/Cython/Cython<3.0.0/g' pyproject.toml"

  command "#{install_dir}/embedded/bin/pip2 install ."
end
