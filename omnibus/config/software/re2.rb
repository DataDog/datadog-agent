# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "re2"
default_version "2025-11-05"

build do
  license "BSD-3-Clause"
  license_file "https://raw.githubusercontent.com/google/re2/main/LICENSE"

  command_on_repo_root "bazelisk run -- @re2//:install --destdir='#{install_dir}'"
  command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
    " #{install_dir}/embedded/lib/libre2.so"

  # Pre-compile the CRE2 C wrapper into a static library so that CGo only
  # needs to link it, avoiding the need for RE2/Abseil headers at Go build time.
  command_on_repo_root "bazelisk build //pkg/logs/re2/internal/cre2:cre2"
  command_on_repo_root "cp $(bazelisk info bazel-bin)/pkg/logs/re2/internal/cre2/libcre2.a" \
    " '#{install_dir}/embedded/lib/'"
end
