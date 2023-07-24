# We need to build the pydantic-core wheel separately because this is the only way to build
# it on CentOS 6.
# manylinux2014 wheels (min. requirement: glibc 2.17) do not support CentOS 6 (glibc 2.12).

name "pydantic-core-py3"
default_version "2.0.1"

dependency "pip3"

source :url => "https://github.com/pydantic/pydantic-core/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "2595fb6e4554af6c4cdf7712b10fb2f1be254955d34fd449038a7d246f6cf1f4",
       :extract => :seven_zip

relative_path "pydantic-core-#{version}"

build do
  license "MIT"
  license_file "./LICENSE"

  patch :source => "pydantic-core-build-for-manylinux1.patch"

  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
  else
    pip = "#{install_dir}/embedded/bin/pip3"
  end

  command "#{pip} install --no-deps ."
end
