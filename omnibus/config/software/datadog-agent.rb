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

build do
  ship_license 'https://raw.githubusercontent.com/DataDog/dd-agent/master/LICENSE'
  command 'rake agent:build'
  copy('bin', install_dir)

  mkdir "#{install_dir}/run/"

  if linux?
    # Config
    mkdir '/etc/dd-agent'
    move 'bin/agent/dist/datadog.yaml', '/etc/dd-agent/datadog.yaml.example'
    mkdir '/etc/dd-agent/checks.d'

    if debian?
      erb source: "upstart.conf.erb",
          dest: "/etc/init/datadog-agent6.conf",
          mode: 0755,
          vars: { install_dir: install_dir }
    end

    if redhat? || debian?
      erb source: "systemd.service.erb",
          dest: "/lib/systemd/system/datadog-agent6.service",
          mode: 0755,
          vars: { install_dir: install_dir }
    end
  end

  if windows?
    copy "pkg/collector/dist/conf.d/*", "../../extra_package_files/EXAMPLECONFSLOCATION"
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
