name 'dogstatsd'

source path: '..'

relative_path 'dogstatsd'

build do
  ship_license 'https://raw.githubusercontent.com/DataDog/dd-agent/master/LICENSE'
  # the go deps needs to be installed (invoke dep) before running omnibus
  # TODO: enable omnibus to run invoke deps while building the project
  command 'invoke dogstatsd.build --rebuild --use-embedded-libs'
  copy('bin', install_dir)

  if debian?
    erb source: "upstart.conf.erb",
        dest: "/etc/init/dogstatsd.conf",
        mode: 0755,
        vars: { install_dir: install_dir }
    erb source: "systemd.service.erb",
        dest: "/lib/systemd/system/dogstatsd.service",
        mode: 0755,
        vars: { install_dir: install_dir }
  end

  if redhat?
    erb source: "systemd.service.erb",
        dest: "/lib/systemd/system/dogstatsd.service",
        mode: 0755,
        vars: { install_dir: install_dir }
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  delete "#{install_dir}/uselessfile"
end
