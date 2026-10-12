# SPDX-FileCopyrightText: 2026-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Change context sent by the datadog-agent-ci DDCI task engine.

The context is the engine's `taskengine.GetTasksRequest` serialized with protojson using the proto field
and enum names, emitting fields holding their default value
(`protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}`). Only these fields are read:

    {
      "request": {
        "kind": "REQUEST_KIND_CHANGE_REQUEST",
        "ref": "my-branch",
        "scheduled_origin": false,
        "repository": {"default_branch": "main"}
      },
      "bia_analysis": {
        "changed_files": [{"path": "doc/index.md", "kind": "FILE_MODIFICATION_KIND_EDITED"}],
        "changed_files_missing": false,
        "large_scale_change": false
      }
    }

`changed_files_missing` and `large_scale_change` must be explicitly `false`: an absent value is unknown.
Unset proto3 `optional` fields such as `changed_files_missing` are omitted even with `EmitDefaultValues`.
"""

from __future__ import annotations

import re
from typing import Any

CHANGE_REQUEST = "REQUEST_KIND_CHANGE_REQUEST"
RENAMED = "FILE_MODIFICATION_KIND_RENAMED"

DOC_EXTENSIONS = (".md", ".markdown", ".rst", ".adoc")
DOC_DIRECTORIES = ("doc/", "docs/")
# Documentation files under these directories are shipped in packages and images, or read by the pipeline
CODE_DIRECTORIES = ("packages/", "omnibus/", "Dockerfiles/", "deps/", ".gitlab/")
RELEASE_BRANCH = re.compile(r"^\d+\.\d+\.x$")


def is_no_code_change(context: dict[str, Any]) -> bool:
    """Whether the change only modifies documentation, so the pipeline can be skipped.

    Anything uncertain (missing analysis, protected branch, unknown file) counts as a code change.
    """
    request = context.get("request") or {}
    if request.get("kind") != CHANGE_REQUEST or request.get("scheduled_origin"):
        return False

    ref = request.get("ref", "")
    if ref.startswith("refs/tags/"):
        return False
    branch = ref.removeprefix("refs/heads/")
    default_branch = (request.get("repository") or {}).get("default_branch") or "main"
    if branch == default_branch or RELEASE_BRANCH.match(branch) or branch.startswith("mq-working-branch-"):
        return False

    analysis = context.get("bia_analysis") or {}
    if analysis.get("changed_files_missing") is not False or analysis.get("large_scale_change") is not False:
        return False

    files = analysis.get("changed_files") or []
    # The previous path of a renamed file is not part of the context
    return bool(files) and all(f.get("kind") != RENAMED and is_documentation(f.get("path", "")) for f in files)


def is_documentation(path: str) -> bool:
    if not path or path.startswith(CODE_DIRECTORIES):
        return False
    return path.startswith(DOC_DIRECTORIES) or path.lower().endswith(DOC_EXTENSIONS)
