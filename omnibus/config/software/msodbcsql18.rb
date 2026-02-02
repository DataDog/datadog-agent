name "msodbcsql18"
default_version "18.3.3.1-1"

dependency "libkrb5"
dependency "unixodbc"

license "MICROSOFT SOFTWARE LICENSE"
license_file "doc/msodbcsql18/LICENSE.txt"
skip_transitive_dependency_licensing true

build do
  # Select the appropriate Bazel install target based on the platform
  if debian_target?
    install_target = "//deps/msodbcsql18:install_deb"
  elsif redhat_target?
    install_target = "//deps/msodbcsql18:install_rpm"
  else
    # SLES
    install_target = "//deps/msodbcsql18:install_rpm_sles"
  end

  # Install the extracted files
  command_on_repo_root "bazelisk run -- #{install_target} --destdir='#{install_dir}'"

  # Fix rpath to point to the embedded lib directory
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " '#{install_dir}/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.3.1'"
end
