name "unixodbc"

build do
  command_on_repo_root "bazelisk run -- @unixodbc//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libodbc.so" \
    " #{install_dir}/embedded/lib/libodbccr.so" \
    " #{install_dir}/embedded/lib/libltdl.so" \
    " #{install_dir}/embedded/lib/libodbcinst.so"
end
