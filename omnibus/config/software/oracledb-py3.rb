name "oracledb-py3"
default_version "1.3.2"

dependency "pip3"

# The github repository contains a submodule which is not shipped with their released file.
# Grabbing the tar from PyPi instead
source :url => "https://files.pythonhosted.org/packages/05/18/516f38a55c99d8ac417a817afff88f484300ce7e4d94058055af1f1461e5/oracledb-#{version}.tar.gz",
       :sha256 => "bb3c391c167b5778ddb15a7538a2b36db5c9b88a50c86c61781ca9ff302bb643",
       :extract => :seven_zip

relative_path "oracledb-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"

  command "sed -i 's/cython/cython<3.0.0/g' pyproject.toml"
  command "sed -i 's/cryptography>=3.2.1/cryptography>=3.2.1,<42.0.0/g' setup.cfg"

  command "#{install_dir}/embedded/bin/pip3 install ."
end
