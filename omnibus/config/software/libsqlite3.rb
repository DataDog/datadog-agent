name "libsqlite3"
default_version "3.50.4"

relative_path "sqlite-autoconf-3500400"

build do
  license "Public-Domain"

  command_on_repo_root "bazelisk run -- @sqlite3//:install --destdir='#{install_dir}/embedded'"
  sh_lib = if linux_target? then "libsqlite3.so" else "libsqlite3.dylib" end
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' " \
     "#{install_dir}/embedded/lib/#{sh_lib}"
end
