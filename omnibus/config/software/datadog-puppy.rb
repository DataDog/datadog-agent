name 'datadog-puppy'
require './lib/ostools.rb'
require 'pathname'

whitelist_file ".*"  # temporary hack, TODO: build libz with omnibus

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  ship_license 'https://raw.githubusercontent.com/DataDog/dd-agent/master/LICENSE'

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  etc_dir = "/etc/datadog-agent"
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
  }
  # include embedded path (mostly for `pkg-config` binary)
  env = with_embedded_path(env)

  command "invoke agent.build --puppy --rebuild --no-development", env: env
  copy('bin', install_dir)

  mkdir "#{install_dir}/run/"

  if linux?
    # Config
    mkdir '/etc/datadog-agent'
    mkdir "/var/log/datadog"

    move 'bin/agent/dist/datadog.yaml', '/etc/datadog-agent/datadog.yaml.example'
    move 'bin/agent/dist/conf.d', '/etc/datadog-agent/'

    if debian?
      erb source: "upstart.conf.erb",
          dest: "/etc/init/datadog-agent.conf",
          mode: 0644,
          vars: { install_dir: install_dir }
    end

    if redhat? || debian? || suse?
      erb source: "systemd.service.erb",
          dest: "/lib/systemd/system/datadog-agent.service",
          mode: 0644,
          vars: { install_dir: install_dir }
    end
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
