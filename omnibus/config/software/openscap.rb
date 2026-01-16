# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'openscap'
default_version '1.4.2'

license "LGPL-3.0-or-later"
license_file "COPYING"

version("1.4.2") { source sha256: "1d5309fadd9569190289d7296016dc534594f7f7d4fd870fe9e847e24940073d" }

ship_source_offer true

source url: "https://github.com/OpenSCAP/openscap/releases/download/#{version}/openscap-#{version}.tar.gz"

build do
  command_on_repo_root "bazelisk run -- @acl//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/libacl.pc" \
    " #{install_dir}/embedded/lib/libacl.so"

  command_on_repo_root "bazelisk run -- @attr//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/libattr.pc" \
    " #{install_dir}/embedded/lib/libattr.so"

  command_on_repo_root "bazelisk run -- @dbus//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/dbus-1.pc"

  command_on_repo_root "bazelisk run -- @libselinux//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/libselinux.pc" \
    " #{install_dir}/embedded/lib/libselinux.so"

  command_on_repo_root "bazelisk run -- @libsepol//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/libsepol.pc" \
    " #{install_dir}/embedded/lib/libsepol.so"

  command_on_repo_root "bazelisk run -- @pcre2//:install --destdir=#{install_dir}/embedded"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix " \
    "--prefix #{install_dir}/embedded " \
    "#{install_dir}/embedded/lib/pkgconfig/libpcre2*.pc " \
    "#{install_dir}/embedded/lib/libpcre2*.so"

  command_on_repo_root "bazelisk run -- @util-linux//:blkid_install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/pkgconfig/blkid.pc" \
    " #{install_dir}/embedded/lib/libblkid.so"

  command_on_repo_root "bazelisk run -- @openscap//:install --destdir='#{install_dir}/embedded'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/openscap.so" \
    " #{install_dir}/embedded/lib/openscap_sce.so"
end
