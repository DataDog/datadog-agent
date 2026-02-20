def parse_go_work(rctx, should_exclude = lambda _: False):
    """Parses use (...) blocks from the go.work file of a repository rule context.

    Args:
        rctx: repository rule context with a go_work label attribute.
        should_exclude: predicate; paths for which it returns True are excluded.

    Returns:
        lines: go.work lines with excluded paths removed.
        paths: set of included module directory paths.
    """
    lines = []
    paths = set()
    in_use_block = 0
    for line in rctx.read(rctx.attr.go_work).splitlines():
        stripped = line.strip()
        if stripped == "use (":
            in_use_block += 1
        elif stripped == ")" and in_use_block:
            in_use_block -= 1
        elif in_use_block and stripped and not stripped.startswith("//"):
            if should_exclude(stripped):
                continue
            paths.add(stripped)
        lines.append(line)
    return struct(lines = lines, paths = paths)
