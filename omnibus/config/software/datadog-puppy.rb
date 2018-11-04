name 'datadog-puppy'
require './lib/ostools.rb'

source path: '..'

relative_path 'stackstate-puppy'

whitelist_file ".*"  # temporary hack, TODO: build libz with omnibus

build do
  ship_license 'https://raw.githubusercontent.com/DataDog/dd-agent/master/LICENSE'
  # the go deps needs to be installed (invoke dep) before running omnibus
  # TODO: enable omnibus to run invoke deps while building the project
  command "invoke -e agent.build --puppy --rebuild --use-embedded-libs --no-development"
  copy('bin', install_dir)

  mkdir "#{install_dir}/run/"

  if linux?
    # Config
    mkdir '/etc/stackstate-agent'
    move 'bin/agent/dist/stackstate.yaml', '/etc/stackstate-agent/stackstate.yaml.example'
    mkdir '/etc/stackstate-agent/checks.d'

    if debian?
      erb source: "upstart.conf.erb",
          dest: "/etc/init/stackstate-agent6.conf",
          mode: 0644,
          vars: { install_dir: install_dir }
    end

    if redhat? || debian? || suse?
      erb source: "systemd.service.erb",
          dest: "/lib/systemd/system/stackstate-agent6.service",
          mode: 0644,
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
