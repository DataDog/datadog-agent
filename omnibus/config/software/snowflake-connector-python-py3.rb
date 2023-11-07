# We need to build the snowflake-connector-python wheel separately because this is the only way to build
# it on CentOS 6.
# manylinux2014 wheels (min. requirement: glibc 2.17) do not support CentOS 6 (glibc 2.12). Therefore, when
# installed by pip on CentOS 6, the wheel is manually compiled. This fails because of a pyarrow build dependency
# that fails to build (defined in pyproject.toml), hence the need to have a separate software definition where we
# modify the wheel build.

name "snowflake-connector-python-py3"
default_version "3.4.0"

dependency "pip3"

source :url => "https://github.com/snowflakedb/snowflake-connector-python/archive/refs/tags/v#{version}.tar.gz",
       # We have to provide a checksum because Github doesn't provide those.
       # PyPI does provide checksums, but
       # 1. we can't find permalinks to source distributions
       # 2. we'll likely need to change our patch to work with a source distribution, we don't have time for that yet.
       :sha256 => "0af8e463d2e558eadb0d75d683e2154acf3e10a74510cc06f93c7596ec82323b",
       :extract => :seven_zip

relative_path "snowflake-connector-python-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"

  # This introduces a pyarrow dependency that is not needed for the agent and fails to build on the CentOS 6 builder.
  delete "pyproject.toml"

  if windows_target?
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

  # We need a newer version of oscrypto than the one released to get a fix for a bug
  # that gets triggered when having double digits on the OpenSSL version
  # (https://github.com/wbond/oscrypto/issues/75).
  # We can remove the oscrypto pinning once the fix becomes part of a new release
  oscrypto_commit = "d5f3437ed24257895ae1edd9e503cfb352e635a8"

  # Adding pyopenssl==23.3.0 here is a temporary workaround so that we don't get
  # conflict because of the `cryptography` version we ship with the agent.
  command "#{pip} install pyopenssl==23.3.0 . \"oscrypto @ git+https://github.com/wbond/oscrypto.git@#{oscrypto_commit}\"", :env => build_env
end
