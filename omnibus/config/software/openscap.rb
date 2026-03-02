# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'openscap'
default_version '1.4.3'

build do
  command_on_repo_root "bazelisk run -- //deps/openscap:install --destdir='#{install_dir}'"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libacl.so"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libattr.so"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libselinux.so"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libsepol.so"

  sh_lib = if linux_target? then "libyaml.so" else "libyaml.dylib" end
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' " \
    "#{install_dir}/embedded/lib/#{sh_lib}"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix " \
    "--prefix #{install_dir}/embedded " \
    "#{install_dir}/embedded/lib/libpcre2*.so"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libpopt.so"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/librpm.so" \
    " #{install_dir}/embedded/lib/librpmio.so"

  command_on_repo_root "bazelisk run -- @util-linux//:blkid_install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libblkid.so"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libgcrypt.so" \
    " #{install_dir}/embedded/lib/libgpg-error.so" \

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libxml2.so"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libxslt.so" \
    " #{install_dir}/embedded/lib/libexslt.so"

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libxmlsec1*.so" \

  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libopenscap.so" \
    " #{install_dir}/embedded/lib/libopenscap_sce.so" \
    " #{install_dir}/embedded/bin/oscap" \
    " #{install_dir}/embedded/bin/oscap-io"

end
