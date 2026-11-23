from __future__ import annotations

from pathlib import Path, PurePosixPath

import yaml


def validate_navigation_roots(config_path: Path) -> None:
    # Leave Zensical's custom Python tags inert when reading navigation settings.
    config = yaml.load(config_path.read_text(encoding="utf-8"), Loader=yaml.BaseLoader)
    if not isinstance(config, dict):
        raise ValueError(f"{config_path}: expected a configuration mapping")

    extra = config.get("extra", {})
    if not isinstance(extra, dict):
        raise ValueError(f"{config_path}: extra must be a mapping")
    roots = extra.get("navigation_roots", [])
    if not isinstance(roots, list):
        raise ValueError(f"{config_path}: extra.navigation_roots must be a list")
    if not roots:
        return

    docs_dir = config_path.parent / config.get("docs_dir", "docs")
    directory_urls = config.get("use_directory_urls", "true").lower() not in {"false", "no", "off"}
    landing_pages: dict[str, list[Path]] = {}
    for entry in config.get("nav", []):
        if not isinstance(entry, dict):
            continue
        for children in entry.values():
            if not isinstance(children, list) or not children:
                continue
            first = children[0]
            if isinstance(first, dict) and len(first) == 1:
                first = next(iter(first.values()))
            if not isinstance(first, str):
                continue
            source = PurePosixPath(first)
            if source.suffix not in {".md", ".markdown"} or source.is_absolute() or ":" in first:
                continue

            # Zensical renders README pages as index pages and supports both URL styles.
            output = source.with_name("index.html") if source.stem == "README" else source.with_suffix(".html")
            if directory_urls:
                route = output.parent if output.name == "index.html" else output.with_suffix("")
                url = f"{route.as_posix()}/" if route.parts else ""
            else:
                url = output.as_posix()
            landing_pages.setdefault(url, []).append(docs_dir / first)

    seen = set()
    for root in roots:
        url = root.get("url") if isinstance(root, dict) else None
        if not isinstance(url, str) or not url:
            raise ValueError(f"{config_path}: each navigation root must specify a nonempty url")
        if url in seen:
            raise ValueError(f"{config_path}: duplicate navigation root URL {url!r}")
        seen.add(url)

        matches = landing_pages.get(url, [])
        if len(matches) != 1:
            raise ValueError(
                f"{config_path}: navigation root {url!r} must match the first page of exactly one "
                f"top-level navigation section; found {len(matches)}. Check the root URL and section's first entry."
            )
        if not matches[0].is_file():
            raise ValueError(f"{config_path}: navigation root {url!r} has no landing page at {matches[0]}")
