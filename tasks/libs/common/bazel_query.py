"""Streaming Bazel query helper."""

from __future__ import annotations

import json
import shutil
import subprocess
from collections.abc import Callable, Generator


def bazel_query(
    query: str,
    filter_func: Callable[[dict], bool],
    flags: list[str] | None = None,
) -> Generator[dict, None, None]:
    """Run a Bazel query with --output=streamed_jsonproto and yield matching objects.

    Each line of the stream is a JSON-encoded build target proto. The caller
    supplies filter_func to select which objects to yield.

    TODO: Support cquery with --output=streamed_proto.

    Args:
        query: The query expression.
        filter_func: Called for each decoded JSON object; yields the object only if it returns True.
        flags: Additional flags passed to bazel query (e.g. ['-k', '--curses=no']).

    Yields:
        Decoded JSON objects for which filter_func returns True.

    Raises:
        RuntimeError: If bazel is not found on PATH.
    """
    resolved_bazel = shutil.which("bazel")
    if not resolved_bazel:
        raise RuntimeError("bazel not found in PATH")

    cmd = [resolved_bazel, 'query', '--output=streamed_jsonproto'] + list(flags or []) + [query]

    proc = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        encoding='utf-8',
    )
    try:
        for line in proc.stdout:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            if filter_func(obj):
                yield obj
    finally:
        proc.stdout.close()
        proc.wait()
