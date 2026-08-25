from __future__ import annotations

from pathlib import Path


def check_links(app) -> None:
    """Resolve every link of the built documentation, exiting with the checker's status."""
    from utils.docs.deps import LINK_CHECKER

    site_dir = Path("site")
    if not site_dir.is_dir():
        app.abort(f"No documentation to check at `{site_dir}`, run `dda run docs build` first")

    # Isolated so that the checker never enters the environment the documentation is built with, and
    # never resolves to a `lychee` that happens to be installed elsewhere.
    lychee_command = ["lychee", "--config", ".lychee.toml", str(site_dir)]
    app.tools.uv.exit_with(["tool", "run", "--isolated", "--from", LINK_CHECKER, *lychee_command])
