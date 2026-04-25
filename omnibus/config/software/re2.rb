# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "re2"
default_version "2025-11-05"

build do
  license "BSD-3-Clause"
  license_file "https://raw.githubusercontent.com/google/re2/main/LICENSE"

  # Build a fat static archive containing cre2 (our C wrapper) plus all of
  # its transitive C++ dependencies (RE2, Abseil). This lets the Go binary
  # link everything statically with no runtime C++ shared-library dependency.
  command_on_repo_root "bazelisk build //pkg/logs/re2/internal/cre2:cre2"
  mkdir "#{install_dir}/embedded/lib"
  command_on_repo_root "cp $(bazelisk info bazel-bin)/pkg/logs/re2/internal/cre2/libcre2.a" \
    " '#{install_dir}/embedded/lib/'"
end
