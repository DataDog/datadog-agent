name 'datadog-agent-integrations-py3-dependencies'

dependency 'python3'
dependency 'setuptools3'

if linux_target?

  build do
    unless heroku_target?
      command_on_repo_root "bazelisk run --//:install_dir=#{install_dir} -- //deps/msodbcsql18:install --destdir='#{install_dir}'"
      # TODO(agent-build): handle msodbcsql18 rpath rewriting in Bazel and drop this recipe entirely.
      command_on_repo_root "bazelisk run --//:install_dir=#{install_dir} -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' '#{install_dir}/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.3.1'"
    end
  end
end
