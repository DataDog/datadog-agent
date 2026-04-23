from __future__ import annotations

import json


def parse_frontmatter(text: str) -> tuple[dict[str, object], str]:
    if not text.startswith("---\n"):
        return {}, text

    parts = text.split("\n---\n", 1)
    if len(parts) != 2:
        return {}, text

    raw_meta = parts[0].splitlines()[1:]
    body = parts[1]
    meta: dict[str, object] = {}
    current_key: str | None = None
    current_list: list[str] | None = None

    for line in raw_meta:
        if not line.strip():
            continue
        if line.startswith("  - "):
            if current_key is None or current_list is None:
                continue
            current_list.append(_parse_scalar(line[4:].strip()))
            continue

        current_key = None
        current_list = None
        key, _, value = line.partition(":")
        key = key.strip()
        value = value.strip()
        if not key:
            continue
        if value == "":
            current_key = key
            current_list = []
            meta[key] = current_list
            continue
        meta[key] = _parse_scalar(value)
    return meta, body


def dump_frontmatter(meta: dict[str, object]) -> str:
    lines = ["---"]
    for key, value in meta.items():
        if isinstance(value, list):
            lines.append(f"{key}:")
            for item in value:
                lines.append(f"  - {json.dumps(item)}")
        else:
            lines.append(f"{key}: {json.dumps(value)}")
    lines.append("---")
    return "\n".join(lines)


def _parse_scalar(value: str) -> object:
    value = value.strip()
    if not value:
        return ""
    if value[0] in {'"', "'"}:
        quote = value[0]
        if quote == "'":
            return value.strip("'")
        return json.loads(value)
    if value in {"true", "false"}:
        return value == "true"
    return value
