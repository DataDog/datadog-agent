import hashlib
import json
import os
import re
import stat
import subprocess
import sys
import types
from functools import cache
from pathlib import Path
from urllib.parse import quote

# Keeping remote content between builds lets one succeed while GitHub is unreachable. Zensical
# empties `.cache` whenever it builds with `--clean`, hence a directory of our own.
FETCH_CACHE_DIR = Path(".docs-cache")

# GitHub line anchors, e.g. `#L12` or `#L12-L20`, capturing the bounds for the range check.
LINE_ANCHOR = re.compile(r"^L([1-9][0-9]*)(?:-L([1-9][0-9]*))?$")
# Zensical re-executes this module for every page, so `functools.cache` cannot hold anything across
# pages while a module in `sys.modules` can. Only state that would otherwise cost a subprocess per
# page belongs here, as caching a value read from a file would defeat the `watch` entries in mkdocs.yml.
CACHE = sys.modules.setdefault("_datadog_agent_docs", types.ModuleType("_datadog_agent_docs")).__dict__


class RepoPathError(Exception):
    pass


@cache
def variable_replacements():
    return {
        "GO_VERSION": get_go_version(),
        "PYTHON_VERSION": get_python_version(),
        "DDA_DOCS_INSTALL": get_dda_install_docs(),
        "DDA_DOCS_TAB_COMPLETE": get_dda_tab_complete_docs(),
        "VSCODE_EXTENSIONS": get_vscode_extensions(),
    }


def fetch_text(url):
    """Return the body of a URL, reading from and writing to the on-disk cache."""
    import httpx

    entry = FETCH_CACHE_DIR / hashlib.sha256(url.encode("utf-8")).hexdigest()
    # Every URL fetched here is addressed by a commit or a tag, so a cached body never goes stale;
    # anything fetched from a moving ref would need an expiry.
    if entry.is_file():
        # Bytes, since text mode would rewrite line endings and make the body differ from the response.
        return entry.read_bytes().decode("utf-8")

    response = httpx.get(url)
    response.raise_for_status()
    if not response.text:
        # Refused rather than cached, since the key never rotates and every consumer parses the body.
        raise RuntimeError(f"Empty response from {url}")

    FETCH_CACHE_DIR.mkdir(parents=True, exist_ok=True)
    # Staged under a unique name so that an interrupted write cannot leave a truncated entry behind.
    staged = entry.with_name(f"{entry.name}.{os.getpid()}")
    staged.write_bytes(response.text.encode("utf-8"))
    os.replace(staged, entry)
    return response.text


@cache
def get_build_image_ref():
    """Return the commit of the build images the Agent is built with."""
    # Pinning to this rather than to `main` documents what developers actually get, and keeps every
    # URL derived from it immutable.
    gitlab_config = Path(".gitlab-ci.yml").read_text(encoding="utf-8")
    # Split the same way as .github/actions/install-dda, which takes everything after the last hyphen.
    build_image_ref = re.search(r"^\s*CI_IMAGE_LINUX: v.*-(.+)$", gitlab_config, flags=re.MULTILINE)
    if build_image_ref is None:
        raise RuntimeError("Unable to find CI_IMAGE_LINUX in .gitlab-ci.yml")

    return build_image_ref.group(1)


@cache
def get_dda_version():
    version_url = f"https://raw.githubusercontent.com/DataDog/datadog-agent-buildimages/{get_build_image_ref()}/dda.env"
    return re.search(r"DDA_VERSION=v(.*)", fetch_text(version_url)).group(1)


def get_go_version():
    return Path(".go-version").read_text(encoding="utf-8").strip()


def get_python_version():
    return Path(".python-version").read_text(encoding="utf-8").strip()


def get_dda_install_docs():
    version = get_dda_version()
    docs_url = f"https://raw.githubusercontent.com/DataDog/datadog-agent-dev/refs/tags/v{version}/docs/install.md"
    # Split out the content from the title divider
    content = fetch_text(docs_url).split("-----", 1)[1]
    # Locate the upgrade section
    upgrade_block = re.search(r"## Upgrade.+?(/// warning)", content, flags=re.DOTALL)
    # Strip out everything after the upgrade section, and ignore its warning
    content = content[: upgrade_block.start(1)]
    # Substitute placeholder with the pinned version
    content = content.replace("<<<DDA_VERSION>>>", version)
    # Add an extra level to the headers
    return re.sub(r"^#", "##", content, flags=re.MULTILINE).strip()


def get_dda_tab_complete_docs():
    version = get_dda_version()
    docs_url = (
        f"https://raw.githubusercontent.com/DataDog/datadog-agent-dev/refs/tags/v{version}/docs/reference/cli/index.md"
    )
    # Extract the tab completion section
    content = re.search(r"^## Tab completion(.+?)(?=^#|\Z)", fetch_text(docs_url), flags=re.MULTILINE | re.DOTALL)
    return content.group(1).strip()


def get_vscode_extensions():
    url = f"https://raw.githubusercontent.com/DataDog/datadog-agent-buildimages/{get_build_image_ref()}/dev-envs/linux/default-vscode-extensions.json"
    marketplace_url_base = "https://marketplace.visualstudio.com/items?itemName="
    return "\n".join(f"- [{extension}]({marketplace_url_base}{extension})" for extension in json.loads(fetch_text(url)))


def source_lines(path):
    # Newlines alone, the way GitHub numbers blob lines. Decoding is lenient because lines are only
    # counted and searched.
    lines = path.read_bytes().decode("utf-8", "replace").split("\n")
    # A trailing newline ends the last line rather than starting another one.
    if lines and not lines[-1]:
        lines.pop()
    # .gitattributes keeps a carriage return in files such as `*.bat`, which would stop a `$`
    # anchored expression from ever matching.
    return [line[:-1] if line.endswith("\r") else line for line in lines]


def git_output(root, *args):
    """Return the output of a Git command, or an empty string when it is unavailable."""
    try:
        # Decoded leniently because the locale encoding cannot represent every branch name.
        result = subprocess.run(
            ("git", *args), capture_output=True, check=True, cwd=root, encoding="utf-8", errors="replace"
        )
    except (OSError, subprocess.CalledProcessError):
        # A tree without Git history still builds, falling back to the default branch.
        return ""
    return result.stdout.strip()


def head_file(root):
    """Return the path of HEAD, following the pointer that a linked worktree stores instead."""
    if "head_file" not in CACHE:
        git_dir = root / ".git"
        if git_dir.is_file():
            git_dir = Path(git_dir.read_text(encoding="utf-8").partition("gitdir:")[2].strip())
        CACHE["head_file"] = git_dir / "HEAD"
    return CACHE["head_file"]


def repo_ref(root):
    """Return the ref being built, so that a preview of a pushed branch links to its own files."""
    head = head_file(root)
    # Keyed by HEAD so that a branch switch while serving is picked up, without paying a subprocess
    # on every page.
    stamp = head.stat().st_mtime_ns if head.is_file() else 0
    if CACHE.get("ref_stamp") != stamp:
        # Covers a build where Git cannot answer, such as one from an archive rather than a
        # checkout. The repository needs no equivalent, as `repo_url` in mkdocs.yml always answers.
        ref = os.environ.get("DOCS_REF", "").strip()
        if not ref:
            # Detached checkouts, such as the merge refs pull requests are built from, report no
            # branch name, and then the commit is the only ref that resolves.
            ref = git_output(root, "branch", "--show-current") or git_output(root, "rev-parse", "HEAD")
        # Escaped because a branch name may contain characters that are reserved in a URL.
        CACHE["ref"] = quote(ref, safe="/") or "main"
        CACHE["ref_stamp"] = stamp
    return CACHE["ref"]


def casing_mismatch(root, parts):
    """Return the first path component whose casing differs from disk, with what disk says."""
    # A case-insensitive file system accepts the wrong casing, which then 404s on GitHub. Comparing
    # against the directory listing is the only check that behaves the same on every platform.
    current = root
    for part in parts:
        names = os.listdir(current)
        if part not in names:
            actual = next((name for name in names if name.lower() == part.lower()), part)
            return f"{part} -> {actual}"
        current = current / part
    return ""


def resolve_repo_path(root, docs_dir, ref, page, path, match):
    """Validate a repository path, returning the URL path GitHub serves it at."""
    location, separator, anchor = path.partition("#")
    anchored = bool(separator)

    def fail(message):
        # Zensical does not report which page failed to render, so name it here.
        return RepoPathError(f"{page}: {message}")

    if not location:
        raise fail(f"empty repository path: {path!r}")
    if location != location.strip() or "\\" in location or ":" in location:
        raise fail(f"path must be relative to the repository root, using forward slashes: {path!r}")
    parts = location.split("/")
    if any(part in ("", ".", "..") for part in parts):
        raise fail(f"path must be normalized, without leading, trailing or relative segments: {path!r}")
    if anchored and match is not None:
        raise fail(f"pass either a line anchor or `match`, not both: {path!r}")
    bounds = LINE_ANCHOR.match(anchor) if anchored else None
    if anchored and bounds is None:
        raise fail(f"line anchor must look like L12 or L12-L20: {path!r}")

    target = root / location
    try:
        # Never follow symlinks: GitHub serves them as blobs even when they point at directories.
        entry = target.lstat()
    except OSError:
        raise fail(f"path does not exist: {location}") from None

    if mismatch := casing_mismatch(root, parts):
        raise fail(f"path casing differs from disk: {mismatch}")

    # Percent-encoded because a tracked path may contain characters that are reserved in a URL.
    encoded = quote(location, safe="/")

    if stat.S_ISDIR(entry.st_mode):
        if anchored or match is not None:
            raise fail(f"line anchors require a file: {location}")
        return f"tree/{ref}/{encoded}"

    if location.endswith(".md") and target.is_relative_to(docs_dir):
        raise fail(f"link documentation pages relatively rather than through GitHub: {location}")

    if stat.S_ISLNK(entry.st_mode) and (anchored or match is not None):
        # GitHub serves the symlink itself, whose only line is the path it points at.
        raise fail(f"line anchors require a regular file rather than a symlink: {location}")

    if match is not None:
        try:
            # Unanchored, so `^` and `$` are opt-in and bind to the line rather than the file.
            pattern = re.compile(match)
        except re.error as error:
            raise fail(f"invalid `match` expression {match!r}: {error}") from None
        hits = [number for number, line in enumerate(source_lines(target), 1) if pattern.search(line)]
        if len(hits) != 1:
            found = f" (lines {', '.join(map(str, hits))})" if hits else ""
            raise fail(f"{location} must have exactly one line matching {match!r}, found {len(hits)}{found}")
        anchor = f"L{hits[0]}"
        anchored = True
    elif anchored:
        start, end = int(bounds.group(1)), int(bounds.group(2) or bounds.group(1))
        if end < start:
            raise fail(f"line anchor must not run backwards: {path!r}")
        total = len(source_lines(target))
        if end > total:
            raise fail(f"{location} has {total} lines, but the anchor refers to line {end}")

    fragment = f"#{anchor}" if anchored else ""
    return f"blob/{ref}/{encoded}{fragment}"


def define_env(env):
    from jinja2 import pass_context

    env.variables.update(variable_replacements())

    # Zensical resolves the configuration file to an absolute path, so this is the repository root.
    root = Path(env.conf["root_dir"]).resolve()
    docs_dir = (root / env.conf["docs_dir"]).resolve()
    # A fork changes `repo_url` in mkdocs.yml, which moves these links and `edit_uri` together.
    base_url = env.conf["repo_url"].rstrip("/")
    ref = repo_ref(root)

    def resolve(context, path, match):
        page = getattr(context.get("page"), "path", "<unknown page>")
        return resolve_repo_path(root, docs_dir, ref, page, path, match)

    @env.macro
    @pass_context
    def repo(context, path, text=None, *, match=None):
        """Render a Markdown link to a path in this repository."""
        label = f"`{path.partition('#')[0]}`" if text is None else text
        return f"[{label}]({base_url}/{resolve(context, path, match)})"

    @env.macro
    @pass_context
    def repo_url(context, path, *, match=None):
        """Render the GitHub URL for a path in this repository."""
        return f"{base_url}/{resolve(context, path, match)}"
