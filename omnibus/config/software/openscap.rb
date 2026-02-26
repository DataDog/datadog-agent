# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'openscap'
default_version '1.4.3'

license "LGPL-3.0-or-later"
license_file "COPYING"

version("1.4.3") { source sha256: "96ebe697aafc83eb297a8f29596d57319278112467c46e6aaf3649b311cf8fba" }

ship_source_offer true

source url: "https://github.com/OpenSCAP/openscap/releases/download/#{version}/openscap-#{version}.tar.gz"

build do
  command_on_repo_root "bazelisk run -- @acl//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libacl.so"

  command_on_repo_root "bazelisk run -- @attr//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libattr.so"

  command_on_repo_root "bazelisk run -- @dbus//:install --destdir='#{install_dir}'"

  command_on_repo_root "bazelisk run -- @libselinux//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libselinux.so"

  command_on_repo_root "bazelisk run -- @libsepol//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libsepol.so"

  command_on_repo_root "bazelisk run -- @libyaml//:install --destdir='#{install_dir}'"
  sh_lib = if linux_target? then "libyaml.so" else "libyaml.dylib" end
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded' " \
    "#{install_dir}/embedded/lib/#{sh_lib}"

  command_on_repo_root "bazelisk run -- @pcre2//:install --destdir=#{install_dir}"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix " \
    "--prefix #{install_dir}/embedded " \
    "#{install_dir}/embedded/lib/libpcre2*.so"

  command_on_repo_root "bazelisk run -- @popt//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libpopt.so"

  command_on_repo_root "bazelisk run -- @rpm//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/librpm.so" \
    " #{install_dir}/embedded/lib/librpmio.so"

  command_on_repo_root "bazelisk run -- @util-linux//:blkid_install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libblkid.so"

  command_on_repo_root "bazelisk run -- @gpg-error//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- @gcrypt//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libgcrypt.so" \
    " #{install_dir}/embedded/lib/libgpg-error.so" \

  command_on_repo_root "bazelisk run -- @libxml2//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libxml2.so"

  command_on_repo_root "bazelisk run -- @libxslt//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libxslt.so" \
    " #{install_dir}/embedded/lib/libexslt.so"

  command_on_repo_root "bazelisk run -- @xmlsec//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libxmlsec1*.so" \

  command_on_repo_root "bazelisk run -- @openscap//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libopenscap.so" \
    " #{install_dir}/embedded/lib/libopenscap_sce.so" \
    " #{install_dir}/embedded/bin/oscap" \
    " #{install_dir}/embedded/bin/oscap-io"

end
