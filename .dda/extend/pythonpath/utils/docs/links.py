from __future__ import annotations

import re
from pathlib import Path
from urllib.parse import urlsplit


def _published_site_url(config_path: Path) -> str:
    import yaml

    # BaseLoader leaves Zensical's custom Python tags inert while reading the scalar site URL.
    config = yaml.load(config_path.read_text(encoding="utf-8"), Loader=yaml.BaseLoader)
    site_url = config.get("site_url") if isinstance(config, dict) else None
    if not isinstance(site_url, str):
        raise ValueError(f"`site_url` must be an absolute HTTP(S) base URL in `{config_path}`")

    site_url = site_url.strip()
    parsed_url = urlsplit(site_url)
    if parsed_url.scheme not in {"http", "https"} or not parsed_url.netloc or parsed_url.query or parsed_url.fragment:
        raise ValueError(f"`site_url` must be an absolute HTTP(S) base URL in `{config_path}`")

    return f"{site_url.rstrip('/')}/"


def _lychee_command(site_dir: Path, config_path: Path = Path("mkdocs.yml")) -> list[str]:
    # Validate published-site URLs against this build so newly added pages do not depend on an
    # earlier deployment.
    local_site_url = f"{site_dir.resolve().as_uri()}/"
    published_site_pattern = f"^{re.escape(_published_site_url(config_path))}"
    return [
        "lychee",
        "--config",
        ".lychee.toml",
        "--remap",
        f"{published_site_pattern} {local_site_url}",
        str(site_dir),
    ]


def check_links(app) -> None:
    """Resolve every link of the built documentation, exiting with the checker's status."""
    from utils.docs.deps import LINK_CHECKER

    site_dir = Path("site")
    if not site_dir.is_dir():
        app.abort(f"No documentation to check at `{site_dir}`, run `dda run docs build` first")

    # Isolated so that the checker never enters the environment the documentation is built with, and
    # never resolves to a `lychee` that happens to be installed elsewhere.
    app.tools.uv.exit_with(["tool", "run", "--isolated", "--from", LINK_CHECKER, *_lychee_command(site_dir)])
