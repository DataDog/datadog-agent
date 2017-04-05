name 'datadog-agent'
require './lib/ostools.rb'
dependency 'python'
# Core checks dependencies
unless windows?
  dependency 'net-snmp-lib'
end
# Python check dependencies
# none atm

source path: '..'

relative_path 'datadog-agent'

build_env = {
  "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
}

build do
  ship_license 'https://raw.githubusercontent.com/DataDog/dd-agent/master/LICENSE'
  command 'rake agent:build', :env => build_env
  copy('bin', install_dir)

  if debian?
    erb source: "upstart.conf.erb",
        dest: "/etc/init/datadog-agent6.conf",
        mode: 0755,
        vars: { install_dir: install_dir }
    erb source: "systemd.service.erb",
        dest: "/lib/systemd/system/datadog-agent6.service",
        mode: 0755,
        vars: { install_dir: install_dir }
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
