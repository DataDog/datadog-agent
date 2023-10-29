# We need to build the pydantic-core wheel separately because this is the only way to build
# it on CentOS 6.
# manylinux2014 wheels (min. requirement: glibc 2.17) do not support CentOS 6 (glibc 2.12).

name "pydantic-core-py3"
default_version "2.1.2"

dependency "pip3"

source :url => "https://github.com/pydantic/pydantic-core/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "63c12928b54c8eab426bcbd1d9af005a945ebf9010caa7a9f087ad69cf29cb07",
       :extract => :seven_zip

relative_path "pydantic-core-#{version}"

build do
  license "MIT"
  license_file "./LICENSE"

  patch :source => "pydantic-core-build-for-manylinux1.patch"

  if windows_target?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
  end

  command "#{pip} install --no-deps ."
end
