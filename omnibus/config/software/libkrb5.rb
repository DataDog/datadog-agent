name "libkrb5"
default_version "1.18.3"

version "1.18.3" do
  source url: "https://kerberos.org/dist/krb5/1.18/krb5-1.18.3.tar.gz"
  source sha256: "e61783c292b5efd9afb45c555a80dd267ac67eebabca42185362bee6c4fbd719"
end

relative_path "krb5-#{version}/src"

reconf_env = { "PATH" => "#{install_dir}/embedded/bin:#{ENV["PATH"]}" }

build do
  license "BSD-style"
  license_file "https://raw.githubusercontent.com/krb5/krb5/master/NOTICE"

  cmd = ["./configure",
         "--without-keyutils", # this would require additional deps/system deps, disable it
         "--without-system-verto", # do not prefer libverto from the system, if installed
         "--without-libedit", # we don't want to link with libraries outside of the install dir
         "--disable-static",
         "--prefix=#{install_dir}/embedded"].join(" ")
  env = {
    "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
  }
  command cmd, :env => env
  command "make -j #{workers}", :env => { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }
  command "make install", :env => { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }

  # FIXME: CONDA libs appear to confuse the health checker - manually checked file
  # are properly linked. Must whitelist for build to succeed.
  whitelist_file "#{install_dir}/embedded/lib/krb5/plugins/tls/k5tls.so"
  whitelist_file "#{install_dir}/embedded/lib/krb5/plugins/preauth/pkinit.so"
end
