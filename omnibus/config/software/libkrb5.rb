name "libkrb5"
default_version "1.21.3"

dependency "openssl3"

version "1.21.3" do
  source url: "https://kerberos.org/dist/krb5/1.21/krb5-1.21.3.tar.gz"
  source sha256: "b7a4cd5ead67fb08b980b21abd150ff7217e85ea320c9ed0c6dadd304840ad35"
end

relative_path "krb5-#{version}/src"

reconf_env = { "PATH" => "#{install_dir}/embedded/bin:#{ENV["PATH"]}" }

build do
  license "BSD-style"
  license_file "https://raw.githubusercontent.com/krb5/krb5/master/NOTICE"

  configure_options = ["--without-keyutils", # this would require additional deps/system deps, disable it
         "--without-system-verto", # do not prefer libverto from the system, if installed
         "--without-libedit", # we don't want to link with libraries outside of the install dir
         "--disable-static"
  ]
  env = with_standard_compiler_flags(with_embedded_path)
  configure(*configure_options, :env => env)
  command "make -j #{workers}", :env => { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }
  command "make install", :env => { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }

  # FIXME: CONDA libs appear to confuse the health checker - manually checked file
  # are properly linked. Must whitelist for build to succeed.
  whitelist_file "#{install_dir}/embedded/lib/krb5/plugins/tls/k5tls.so"
  whitelist_file "#{install_dir}/embedded/lib/krb5/plugins/preauth/pkinit.so"
end
