"""
Claude Code namespaced tags

Helpers for getting Claude Code set up nicely with gopls
"""

from __future__ import annotations

import json
import os

from invoke import task

from tasks.build_tags import build_tags, compute_config_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.types.version import Version

CLAUDE_PLUGINS_DIR = ".claude-plugins"
PLUGIN_NAME = "datadog-agent-gopls"
PLUGIN_DIR = os.path.join(CLAUDE_PLUGINS_DIR, PLUGIN_NAME, ".claude-plugin")
PLUGIN_FILE = "plugin.json"
DEFAULT_VERSION = "1.0.0"


def _read_existing_plugin() -> tuple[Version, str | None]:
    """Read the existing plugin.json if it exists, returning (version, build_flags_tag)."""
    fullpath = os.path.join(PLUGIN_DIR, PLUGIN_FILE)
    try:
        with open(fullpath) as f:
            plugin = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return Version.from_tag(DEFAULT_VERSION), None
    version = Version.from_tag(plugin.get("version", DEFAULT_VERSION))
    flags = plugin.get("lspServers", {}).get("gopls", {}).get("initializationOptions", {}).get("build.buildFlags", [])
    tags_flag = next((f for f in flags if f.startswith("-tags=")), None)
    return version, tags_flag


@task(
    help={
        "targets": f"Comma separated list of targets to include. Possible values: all, {', '.join(build_tags[AgentFlavor.base].keys())}. Default: all",
        "flavor": f"Agent flavor to use. Possible values: {', '.join(AgentFlavor.__members__.keys())}. Default: {AgentFlavor.base.name}",
    }
)
def set_buildtags(
    _: object,
    targets: str = "all",
    build_include: str | None = None,
    build_exclude: str | None = None,
    flavor: str = AgentFlavor.base.name,
) -> None:
    """
    Create/update Claude Code gopls plugin configuration with correct build tags
    """
    use_tags = compute_config_build_tags(
        targets=targets,
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=flavor,
    )

    if not os.path.exists(PLUGIN_DIR):
        os.makedirs(PLUGIN_DIR)

    new_tags_flag = f"-tags={','.join(sorted(use_tags))}"
    version, old_tags_flag = _read_existing_plugin()
    if old_tags_flag is not None and old_tags_flag != new_tags_flag:
        version = version.next_version(bump_patch=True)

    plugin = {
        "name": PLUGIN_NAME,
        "version": str(version),
        "description": "Go LSP (gopls) pre-configured for the datadog-agent repository with build tags and performance flags",
        "author": {
            "name": "Lénaïc Huard",
            "email": "lenaic.huard@datadoghq.com",
        },
        "lspServers": {
            "gopls": {
                "command": "gopls",
                "extensionToLanguage": {
                    ".go": "go",
                },
                "initializationOptions": {
                    "build.buildFlags": [
                        new_tags_flag,
                        "-buildvcs=false",
                    ],
                    "formatting.local": "github.com/DataDog/datadog-agent",
                },
            }
        },
    }

    fullpath = os.path.join(PLUGIN_DIR, PLUGIN_FILE)
    with open(fullpath, "w") as f:
        json.dump(plugin, f, indent=2)
        f.write("\n")
