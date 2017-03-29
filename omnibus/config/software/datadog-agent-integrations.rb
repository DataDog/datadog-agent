name 'datadog-agent-integrations'

dependency 'pip'

relative_path 'integrations-core'
whitelist_file "embedded/lib/python2.7"

source git: 'https://github.com/DataDog/integrations-core.git'
default_version 'master'

requirements = %{
  psutil==4.4.1
  simplejson==3.6.5
  scandir==1.5
  dnspython==1.12.0
  gearman==2.0.2
  snakebite==1.3.11
  kafka-python==1.3.1
  kazoo==2.2.1
  python-memcached==1.53
  pymongo==3.2
  pymysql==0.6.6
  ntplib==0.3.3
  psycopg2==2.7.1
  pg8000==1.10.1
  redis==2.10.5
  httplib2==0.9
  pysnmp-mibs==0.1.4
  pyasn1==0.1.9
  pysnmp==4.2.5
  beautifulsoup4==4.5.1
  paramiko==1.15.2
  supervisor==3.3.0
  pyvmomi==6.0.0
}

build do
  # The checks
  checks_dir = "#{install_dir}/bin/agent/dist/checks.d"
  mkdir checks_dir

  # The confs
  if linux?
    conf_directory = "/etc/dd-agent/conf.d"
  elsif osx?
    conf_directory = "#{install_dir}/etc"
  elsif windows?
    conf_directory = "../../extra_package_files/EXAMPLECONFSLOCATION"
  end

  # Copy the checks and generate the global requirements file
  command 'gem install bundle'
  command 'bundle install'
  command "rake copy_checks conf_dir=#{conf_directory} checks_dir=#{checks_dir}"
  command "echo \"#{requirements}\" > checks_requirements.txt"

  # Install all the requirements
  pip_args = "install --install-option=\"--install-scripts=#{windows_safe_path(install_dir)}/bin\" -r checks_requirements.txt"
  if windows?
    command "#{windows_safe_path(install_dir)}\\embedded\\scripts\\pip.exe #{pip_args}"
  else
    build_env = {
      "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
      "PATH" => "/#{install_dir}/embedded/bin:#{ENV['PATH']}",
    }
    command "pip #{pip_args}", :env => build_env
  end
end