name "cython2"
default_version "0.24"

dependency "python2"
dependency "pip2"

build do
  ship_license "https://raw.githubusercontent.com/cython/cython/master/LICENSE.txt"
  command "#{install_dir}/embedded/bin/pip3 install --install-option=\"--no-cython-compile\" cython==#{version}"
end