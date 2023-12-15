name "libkrb5"
default_version "1.20.1"

dependency ENV["OMNIBUS_OPENSSL_SOFTWARE"] || "openssl"

version "1.20.1" do
  source url: "https://kerberos.org/dist/krb5/1.20/krb5-1.20.1.tar.gz"
  source sha256: "704aed49b19eb5a7178b34b2873620ec299db08752d6a8574f95d41879ab8851"
end

version "1.18.3" do
  source url: "https://kerberos.org/dist/krb5/1.18/krb5-1.18.3.tar.gz"
  source sha256: "e61783c292b5efd9afb45c555a80dd267ac67eebabca42185362bee6c4fbd719"
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
