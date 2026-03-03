name 'datadog-agent-integrations-py3-dependencies'

dependency 'python3'
dependency 'setuptools3'
dependency 'openssl3'

if linux_target?

  build do
    command_on_repo_root "bazelisk run -- @unixodbc//:install --destdir='#{install_dir}/embedded'"
    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libodbc.so" \
    " #{install_dir}/embedded/lib/libodbccr.so" \
    " #{install_dir}/embedded/lib/libodbcinst.so"

    command_on_repo_root "bazelisk run -- @freetds//:install --destdir='#{install_dir}'"
    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libtdsodbc.so"
  
    unless heroku_target?
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
      bin_files = [
          'kinit',
        ]
      
      command_on_repo_root "bazelisk run -- //deps/msodbcsql18:install --destdir='#{install_dir}'"
      # (TODO(agent-build): Check if we still need pc files)
      command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' " \
        + lib_files.map{ |l| "#{install_dir}/embedded/lib/#{l}" }.join(' ') \
        + " " \
        + pc_files.map{ |pc| "#{install_dir}/embedded/lib/pkgconfig/#{pc}" }.join(' ') \
        + " " \
        + bin_files.map{ |bin| "#{install_dir}/embedded/bin/#{bin}" }.join(' ') \
        + " '#{install_dir}/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.3.1'"
    end

    # gstatus binary used by the glusterfs integration
    command_on_repo_root "bazelisk run -- //deps/gstatus:install --destdir='#{install_dir}'"
    command_on_repo_root "bazelisk run -- //deps/nfsiostat:install --destdir='#{install_dir}'"
  end
end
