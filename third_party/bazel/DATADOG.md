OWNER: tony.aiuto@
PROJECT: https://datadoghq.atlassian.net/browse/ABLD-158

Temporary clone of Bazel's built in repository rules (#41783)

Copied from https://github.com/bazelbuild/bazel/tree/master/tools/build_defs/repo

This copy brings in some required capabilities to http_archive that are upstreamed but not yet in a Bazel release.

tony.aiuto@ asserts that:
- It includes a copy of the appropriate LICENSE file from upstream Bazel
- No copyright attestation is needed because we use this only at build time and not inside the product.
