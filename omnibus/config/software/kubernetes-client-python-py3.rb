# We need to build the kubernetes python client wheel separately to include a patch that has not been released yet: 
# https://github.com/kubernetes-client/python/pull/2022, which is needed to remove our runtime dependency on setuptools

name "kubernetes-client-python-py3"
default_version "26.1.0"

dependency "pip3"

source :url => "https://files.pythonhosted.org/packages/34/19/2f351c0eaf05234dc33a6e0ffc7894e9dedab0ff341311c5b4ba44f2d8ac/kubernetes-#{version}.tar.gz",
       :sha256 => "5854b0c508e8d217ca205591384ab58389abdae608576f9c9afc35a3c76a366c",
       :extract => :seven_zip

relative_path "kubernetes-#{version}"

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
