from __future__ import annotations

DEPENDENCIES = (
    "mkdocs~=1.6.1",
    "mkdocs-material~=9.6.11",
    # Plugins
    "mkdocs-minify-plugin~=0.8.0",
    # https://github.com/timvink/mkdocs-git-revision-date-localized-plugin/issues/181
    "mkdocs-git-revision-date-localized-plugin~=1.3.0",
    "mkdocs-glightbox~=0.4.0",
    "mkdocs-redirects~=1.2.2",
    "mkdocstrings-python~=1.16.10",
    # Extensions
    "mkdocs-click~=0.9.0",
    "pymdown-extensions~=10.14.3",
    # Fetching data
    "httpx",
    # Necessary for syntax highlighting in code blocks
    "pygments~=2.19.1",
    # Validation
    "linkchecker~=10.5.0",
)
