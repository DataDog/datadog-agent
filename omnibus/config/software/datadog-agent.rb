name 'datadog-agent'
require './lib/ostools.rb'

dependency 'python'
unless windows?
  dependency 'net-snmp-lib'
end

source path: '..'

relative_path 'datadog-agent'

build do
  ship_license 'https://raw.githubusercontent.com/DataDog/dd-agent/master/LICENSE'
  # the go deps needs to be installed (invoke dep) before running omnibus
  # TODO: enable omnibus to run invoke deps while building the project
  command "invoke agent.build --rebuild --use-embedded-libs --no-development"
  copy('bin', install_dir)

  mkdir "#{install_dir}/run/"

  if linux?
    # Config
    mkdir '/etc/dd-agent'
    move 'bin/agent/dist/datadog.yaml', '/etc/dd-agent/datadog.yaml.example'
    mkdir '/etc/dd-agent/checks.d'

    # Change DIRPATH to the absolute path so that a symlink to `bin/agent/agent` works
    command "sed -i -e s@DIRPATH=.*@DIRPATH=#{install_dir}/bin/agent@ #{install_dir}/bin/agent/agent"

    mkdir "/etc/init/"
    if debian? || redhat?
      erb source: "upstart.conf.erb",
          dest: "/etc/init/datadog-agent6.conf",
          mode: 0755,
          vars: { install_dir: install_dir }
    end

    mkdir "/lib/systemd/system/"
    if redhat? || debian?
      erb source: "systemd.service.erb",
          dest: "/lib/systemd/system/datadog-agent6.service",
          mode: 0755,
          vars: { install_dir: install_dir }
    end
  end

  if windows?
    mkdir "../../extra_package_files/EXAMPLECONFSLOCATION"
    copy "pkg/collector/dist/conf.d/*", "../../extra_package_files/EXAMPLECONFSLOCATION"
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
