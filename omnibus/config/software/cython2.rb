name "cython2"
default_version "0.29.21"

dependency "python2"
dependency "pip2"

build do
  if windows?
    pip = "#{windows_safe_path(python_2_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip2"
  end

  ship_license "https://raw.githubusercontent.com/cython/cython/master/LICENSE.txt"
  command "#{pip} install --install-option=\"--no-cython-compile\" cython==#{version}"
end