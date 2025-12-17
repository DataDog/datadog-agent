name "libkrb5"
default_version "1.21.3"

dependency "openssl3"

build do
  pc_files = [
    'gssrpc.pc',
    'kadm-client.pc',
    'kadm-server.pc',
    'kdb.pc',
    'krb5-gssapi.pc',
    'krb5.pc',
    'mit-krb5-gssapi.pc',
    'mit-krb5.pc',
  ]
  lib_files = [
    'krb5/plugins/tls/k5tls.so',
    'krb5/plugins/kdb/db2.so',
    'krb5/plugins/preauth/test.so',
    'krb5/plugins/preauth/spake.so',
    'krb5/plugins/preauth/pkinit.so',
    'krb5/plugins/preauth/otp.so',
    'libkadm5clnt_mit.so',
    'libkrad.so',
    'libverto.so',
    'libk5crypto.so',
    'libcom_err.so',
    'libkadm5srv.so',
    'libkrb5support.so',
    'libgssrpc.so',
    'libkrb5.so',
    'libkadm5srv_mit.so',
    'libkdb5.so',
    'libgssapi_krb5.so',
    'libkadm5clnt.so',
  ]
  command_on_repo_root "bazelisk run -- @krb5//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' " \
    + lib_files.map{ |l| "#{install_dir}/embedded/lib/#{l}" }.join(' ') \
    + " " \
    + pc_files.map{ |pc| "#{install_dir}/embedded/lib/pkgconfig/#{pc}" }.join(' ')
end
