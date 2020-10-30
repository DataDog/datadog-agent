name "cython3"
default_version "0.29.21"

dependency "python3"
dependency "pip3"

build do
  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
  end

  ship_license "https://raw.githubusercontent.com/cython/cython/master/LICENSE.txt"
  command "#{pip} install --install-option=\"--no-cython-compile\" cython==#{version}"
end