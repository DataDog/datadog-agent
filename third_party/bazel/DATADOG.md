OWNER: tony.aiuto@datadoghq.com
PROJECT: https://datadoghq.atlassian.net/browse/ABLD-158

Temporary clone of Bazel's built in repository rules (#41783)

Copied from https://github.com/bazelbuild/bazel/tree/master/tools/build_defs/repo

This copy brings in some required capabilities to http_archive that are upstreamed but not yet in a Bazel release.

tony.aiuto@ asserts that:
- It includes a copy of the appropriate LICENSE file from upstream Bazel
- No copyright attestation is needed because we use this only at build time and not inside the product.


OWNER: joseph.gette@datadoghq.com
PROJECT: https://datadoghq.atlassian.net/browse/ABLD-158

Temporary clone of rules_foreign_cc (#42930)

Copied from https://github.com/bazel-contrib/rules_foreign_cc

This copy contains a new functionality that allows to expose pkgconfig (`.pc`) files
as a separate output group.

joseph.gette@ asserts that:
- It includes a copy of the appropriate LICENSE file from upstream rules_foreign_cc
- No copyright attestation is needed because we use this only at build time and not inside the product.