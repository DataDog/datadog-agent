"""
Streaming JSON parser for bpftool output.

Parses JSON objects one at a time from bpftool map dump output,
enabling O(1) memory usage instead of loading all entries at once.
"""

import json
from typing import Generator, Dict, TextIO

from ..constants import STREAM_CHUNK_SIZE, JSON_ERROR_PREVIEW_LENGTH
from ..logging_config import logger


def iter_json_objects(stream: TextIO) -> Generator[Dict, None, None]:
    """Yield JSON objects one at a time from bpftool --json output.

    bpftool outputs compact JSON arrays like:
        [{"key":...,"value":...},{"key":...,"value":...}]

    This function reads character by character, tracking brace depth
    to extract complete objects without loading the entire output.

    Args:
        stream: Text stream (e.g., stdout from subprocess.Popen)

    Yields:
        Parsed JSON objects (dicts with 'key' and 'value' fields)
    """
    depth = 0
    in_string = False
    escape_next = False
    obj_chars = []
    in_object = False

    for chunk in iter(lambda: stream.read(STREAM_CHUNK_SIZE), ''):
        for char in chunk:
            # Handle string escaping
            if escape_next:
                escape_next = False
                if in_object:
                    obj_chars.append(char)
                continue

            if char == '\\' and in_string:
                escape_next = True
                if in_object:
                    obj_chars.append(char)
                continue

            # Toggle string state on unescaped quotes
            if char == '"':
                in_string = not in_string
                if in_object:
                    obj_chars.append(char)
                continue

            # Skip if inside a string
            if in_string:
                if in_object:
                    obj_chars.append(char)
                continue

            # Track braces outside strings
            if char == '{':
                if not in_object:
                    in_object = True
                    obj_chars = ['{']
                    depth = 1
                else:
                    depth += 1
                    obj_chars.append(char)
            elif char == '}':
                if in_object:
                    depth -= 1
                    obj_chars.append(char)
                    if depth == 0:
                        # Complete object found
                        obj_str = ''.join(obj_chars)
                        try:
                            yield json.loads(obj_str)
                        except json.JSONDecodeError:
                            logger.warning("Failed to parse JSON object: %s...", obj_str[:JSON_ERROR_PREVIEW_LENGTH])
                        obj_chars = []
                        in_object = False
            elif in_object:
                obj_chars.append(char)
