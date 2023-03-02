# We need to build the kubernetes python client wheel separately to include a patch that has not been released yet: 
# https://github.com/kubernetes-client/python/pull/2022, which is needed to remove our runtime dependency on setuptools

name "supervisor-python-py3"
default_version "4.2.5"

dependency "pip3"

source :url => "https://github.com/Supervisor/supervisor/archive/refs/tags/#{version}.tar.gz",
       :sha256 => "d612a48684cf41ea7ce8cdc559eaa4bf9cbaa4687c5aac3f355c6d2df4e4f170",
       :extract => :seven_zip

relative_path "supervisor-#{version}"

build do
  license "BSD-3-Clause"
  license_file "./LICENSES.txt"

  patch :source => "remove-setuptools-confecho.patch", :target => "supervisor/confecho.py"
  patch :source => "remove-setuptools-configuration.patch", :target => "docs/configuration.rst"
  patch :source => "remove-setuptools-events.patch", :target => "docs/events.rst"
  patch :source => "remove-setuptools-options.patch", :target => "supervisor/options.py"
  patch :source => "remove-setuptools-setup.patch", :target => "setup.py"
  patch :source => "remove-setuptools-tests.patch", :target => "supervisor/tests/test_end_to_end.py"
 
  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
  end

  command "#{pip} install ."
end
