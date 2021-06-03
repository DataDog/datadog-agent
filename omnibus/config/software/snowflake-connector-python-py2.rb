name "snowflake-connector-python-py2"

dependency "pip2"

default_version "2.1.3"

source :url => "https://github.com/snowflakedb/snowflake-connector-python/archive/v#{version}.tar.gz",
       :sha256 => "855ffb93a09c3cd994dab8af7c87a46038bbba103928c5948a0edcd2500f4e1a",
       :extract => :seven_zip

relative_path "snowflake-connector-python-#{version}"


build do
  if windows?
    pip = "#{windows_safe_path(python_2_embedded)}\\Scripts\\pip.exe"

    # HACK: replace misbehaving symlinks by real copies of what they
    # linked.
    delete "connector_python2"
    delete "connector_python3"
    copy "test", "connector_python2"
    copy "test", "connector_python3"
  else
    pip = "#{install_dir}/embedded/bin/pip2"
  end

  ship_license "https://raw.githubusercontent.com/snowflakedb/snowflake-connector-python/v#{version}/LICENSE.txt"
  patch :source => "dependencies.patch", :target => "setup.py"
  command "#{pip} install ."
end
