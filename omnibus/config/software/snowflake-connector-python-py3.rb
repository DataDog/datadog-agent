# We need to build the snowflake-connector-python wheel separately because this is the only way to build
# it on CentOS 6.
# manylinux2014 wheels (min. requirement: glibc 2.17) do not support CentOS 6 (glibc 2.12). Therefore, when
# installed by pip on CentOS 6, the wheel is manually compiled. This fails because of a pyarrow build dependency
# that fails to build (defined in pyproject.toml), hence the need to have a separate software definition where we
# modify the wheel build.

name "snowflake-connector-python-py3"
default_version "3.1.0"

dependency "pip3"

source :url => "https://github.com/snowflakedb/snowflake-connector-python/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "fb2477b653bd58edd0366b4d6395d109fd4e238b85ce5685d7944455e0d48dab",
       :extract => :seven_zip

relative_path "snowflake-connector-python-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"

  # This introduces a pyarrow dependency that is not needed for the agent and fails to build on the CentOS 6 builder.
  delete "pyproject.toml"

  if windows?
    pip = "#{windows_safe_path(python_3_embedded)}\\Scripts\\pip.exe"
    build_env = {}
  else
    pip = "#{install_dir}/embedded/bin/pip3"
    build_env = {
      "CFLAGS" => "-I#{install_dir}/embedded/include",
      "CXXFLAGS" => "-I#{install_dir}/embedded/include",
      "LDFLAGS" => "-L#{install_dir}/embedded/lib",
    }
  end

  command "#{pip} install .", :env => build_env
end
