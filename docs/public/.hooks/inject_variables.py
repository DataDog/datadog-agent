import os
import re
import subprocess
from functools import cache
from pathlib import Path

import httpx


@cache
def variable_replacements():
    return {
        variable: replacement
        for variable, replacement in (
            ("GO_VERSION", get_go_version()),
            ("PYTHON_VERSION", get_python_version()),
            ("DDA_DOCS_INSTALL", get_dda_install_docs()),
            ("DDA_DOCS_TAB_COMPLETE", get_dda_tab_complete_docs()),
            ("VSCODE_EXTENSIONS", get_vscode_extensions()),
            ("SRC", get_source_url_base()),
        )
    }


@cache
def get_source_ref():
    # The `dda run docs *` commands and CI set this explicitly; any git ref works
    if ref := os.environ.get("DOCS_SOURCE_REF"):
        return ref

    # Fall back to the current branch for ad-hoc local builds
    try:
        process = subprocess.run(
            ["git", "rev-parse", "--abbrev-ref", "HEAD"],
            capture_output=True,
            text=True,
            check=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return "main"

    branch = process.stdout.strip()
    # A detached HEAD resolves to the literal string `HEAD`
    return branch if branch and branch != "HEAD" else "main"


def get_source_url_base():
    return f"https://github.com/DataDog/datadog-agent/blob/{get_source_ref()}"


@cache
def get_dda_version():
    # TODO: uncomment when the build images get updated
    # gitlab_config = Path(".gitlab-ci.yml").read_text(encoding="utf-8")
    # build_image_ref = re.search(r"^\s*CI_IMAGE_LINUX: v[^-]+-(.+)$", gitlab_config, flags=re.MULTILINE).group(1)
    # version_url = f"https://raw.githubusercontent.com/DataDog/datadog-agent-buildimages/{build_image_ref}/dda.env"
    version_url = "https://raw.githubusercontent.com/DataDog/datadog-agent-buildimages/refs/heads/main/dda.env"
    response = httpx.get(version_url)
    response.raise_for_status()
    return re.search(r"DDA_VERSION=v(.*)", response.text).group(1)


def get_go_version():
    return Path(".go-version").read_text(encoding="utf-8").strip()


def get_python_version():
    return Path(".python-version").read_text(encoding="utf-8").strip()


def get_dda_install_docs():
    version = get_dda_version()
    docs_url = f"https://raw.githubusercontent.com/DataDog/datadog-agent-dev/refs/tags/v{version}/docs/install.md"
    response = httpx.get(docs_url)
    response.raise_for_status()
    # Split out the content from the title divider
    content = response.text.split("-----", 1)[1]
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
    response = httpx.get(docs_url)
    response.raise_for_status()
    # Extract the tab completion section
    content = re.search(r"^## Tab completion(.+?)(?=^#|\Z)", response.text, flags=re.MULTILINE | re.DOTALL)
    return content.group(1).strip()


def get_vscode_extensions():
    url = "https://raw.githubusercontent.com/DataDog/datadog-agent-buildimages/refs/heads/main/dev-envs/linux/default-vscode-extensions.json"
    response = httpx.get(url)
    response.raise_for_status()
    marketplace_url_base = "https://marketplace.visualstudio.com/items?itemName="
    return "\n".join(
        f"- [{extension}]({marketplace_url_base}{extension})"
        for extension in response.json()
    )


def define_env(env):
    env.variables.update(variable_replacements())
