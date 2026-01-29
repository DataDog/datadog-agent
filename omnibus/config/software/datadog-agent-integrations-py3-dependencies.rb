name 'datadog-agent-integrations-py3-dependencies'

dependency 'pip3'
dependency 'setuptools3'
dependency 'openssl3'

if linux_target?
  # odbc drivers used by the SQL Server integration

  dependency "unixodbc"
  unless heroku_target?
    dependency 'msodbcsql18' # needed for SQL Server integration
  end

  build do
    command_on_repo_root "bazelisk run -- @freetds//:install --destdir='#{install_dir}'"
    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libtdsodbc.so"

    # gstatus binary used by the glusterfs integration
    command_on_repo_root "bazelisk run -- //deps/gstatus:install --destdir='#{install_dir}'"
    command_on_repo_root "bazelisk run -- //deps/nfsiostat:install --destdir='#{install_dir}'"
  end
end
