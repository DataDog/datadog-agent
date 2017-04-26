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
  command "echo calling rake"
  command 'rake agent:build', :env => build_env
  command "echo done calling rake"
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
  
  if windows?
    copy('pkg/collector/dist/conf.d/*', '../../extra_package_files/EXAMPLECONFSLOCATION')
    mkdir 'cmd/gui/checks'
    copy('pkg/collector/dist/checks/*', 'cmd/gui/checks')
    command "chdir cmd/gui && C:/opt/datadog-agent6/embedded/python -d setup.py py2exe"
    copy('cmd/gui/dist/*', "#{install_dir}/bin/agent")
    copy('cmd/gui/status.html', "#{install_dir}/bin/agent")
    copy('cmd/gui/guidata', "#{install_dir}/bin/agent/guidata")
  end
  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
if windows?
  dependency 'docker-py'
  dependency 'gui'
  dependency 'kazoo'
  dependency 'ntplib'
  dependency 'psutil'
  dependency 'python-consul'
  dependency 'python-etcd'
  dependency 'pywin32'
  dependency 'py2exe'
  dependency 'pyyaml'
  dependency 'requests'
  dependency 'simplejson'
  dependency 'tornado'
  
  dependency 'datadog-agent-integrations'
end