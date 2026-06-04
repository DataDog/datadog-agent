"""Generate tags.go for the dd_agent_go_test Gazelle extension.

Reads FLAVOR_UNIT_TEST_TAGS from tasks/build_tags.bzl (valid Python: set([...])
literals and set operators) and emits the FlavorUnitTestTags map the extension
uses to decide which per-flavor dd_agent_go_test variants apply to a package.

Usage: gen_tags_go.py <build_tags.bzl> <out.go>
"""

import sys

_HEADER = """// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Code generated from tasks/build_tags.bzl. DO NOT EDIT.

package dd_agent_go_test

// FlavorUnitTestTags maps each agent flavor to its unit-test build tags. The
// LINUX_ONLY tags are included unconditionally: at Gazelle generation time the
// target platform is unknown, and flavor_gotags()'s select() enforces the
// platform restrictions at build time.
var FlavorUnitTestTags = map[string][]string{"""


def main() -> None:
    bzl_path, out_path = sys.argv[1], sys.argv[2]
    namespace: dict = {}
    with open(bzl_path) as src:
        exec(src.read(), namespace)  # noqa: S102 - build_tags.bzl is trusted, valid Python

    flavor_tags = namespace["FLAVOR_UNIT_TEST_TAGS"]
    lines = [_HEADER]
    for flavor in sorted(flavor_tags):
        lines.append(f'\t"{flavor}": {{')
        for tag in flavor_tags[flavor]:
            lines.append(f'\t\t"{tag}",')
        lines.append("\t},")
    lines.append("}\n")

    with open(out_path, "w") as out:
        out.write("\n".join(lines))


if __name__ == "__main__":
    main()
