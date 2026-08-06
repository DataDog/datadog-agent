"""Generate tags.go for the dd_agent_go_test Gazelle extension.

Reads the flavorless test-tag definitions from tasks/build_tags.bzl and emits
the Go values used to derive per-package tag-set variants.

Usage: gen_tags_go.py <build_tags.bzl> <out.go>
"""

import sys

_HEADER = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Code generated from tasks/build_tags.bzl. DO NOT EDIT.

package dd_agent_go_test

"""


def main() -> None:
    bzl_path, out_path = sys.argv[1], sys.argv[2]
    namespace: dict = {}
    with open(bzl_path) as src:
        exec(src.read(), namespace)  # noqa: S102 - build_tags.bzl is trusted, valid Python

    lines = [_HEADER, "// BaseTestTags are applied to every generated Go unit test.", "var BaseTestTags = []string{"]
    for tag in namespace["BASE_TEST_TAGS"]:
        lines.append(f'\t"{tag}",')
    lines.extend(
        ["}", "", "// AutoTestTags may form source-derived test variants.", "var AutoTestTags = map[string]bool{"]
    )
    for tag in namespace["AUTO_TEST_TAGS"]:
        lines.append(f'\t"{tag}": true,')
    lines.append("}\n")

    with open(out_path, "w") as out:
        out.write("\n".join(lines))


if __name__ == "__main__":
    main()
