from __future__ import annotations

DEPENDENCIES = (
    "zensical~=0.0.50",
    # Fetching data
    "httpx",
)

# Kept out of the build's dependencies so that a local build or serve never unpacks a checker it
# does not use.
LINK_CHECKER = "lychee-bin~=0.24.2"
