# We need to build the kubernetes python client wheel separately to include a patch that has not been released yet: 
# https://github.com/kubernetes-client/python/pull/2022, which is needed to remove our runtime dependency on setuptools

name "kubernetes-client-python-py3"
default_version "26.1.0"

dependency "pip3"

source :url => "https://github.com/kubernetes-client/python/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "73bc8e450efb90ac680d8042d3498c873b6aec384db598cd829d8988db14d6f1",
       :extract => :seven_zip

relative_path "python-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE"

  patch :source => "remove-setuptools.patch", :target => "setup.py"
 
  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
  end

  command "#{pip} install ."
end
