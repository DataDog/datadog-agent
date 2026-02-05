name "msodbcsql18"
default_version "18.3.3.1-1"

dependency "libkrb5"
dependency "unixodbc"

license "MICROSOFT SOFTWARE LICENSE"
license_file "doc/msodbcsql18/LICENSE.txt"
skip_transitive_dependency_licensing true

build do
  # Install the extracted files from deb package
  command_on_repo_root "bazelisk run -- //deps/msodbcsql18:install --destdir='#{install_dir}'"

  # Fix rpath to point to the embedded lib directory
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " '#{install_dir}/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.3.1'"
end
