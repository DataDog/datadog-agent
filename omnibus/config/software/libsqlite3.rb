name "libsqlite3"
default_version "3.50.4"

dependency "zlib"

source url: "https://sqlite.org/2025/sqlite-autoconf-3500400.tar.gz",
       sha256: "a3db587a1b92ee5ddac2f66b3edb41b26f9c867275782d46c3a088977d6a5b18"

relative_path "sqlite-autoconf-3500400"

build do
  license "Public-Domain"

  command_on_repo_root "bazelisk run -- @sqlite3//:install --destdir='#{install_dir}/embedded'"
  sh_lib = if linux_target? then "libsqlite3.so" else "libsqlite3.dylib" end
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' " \
     "#{install_dir}/embedded/lib/pkgconfig/sqlite3.pc " \
     "#{install_dir}/embedded/lib/#{sh_lib}"
end
