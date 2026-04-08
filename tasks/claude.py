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

CLAUDE_PLUGINS_DIR = ".claude-plugins"
PLUGIN_NAME = "datadog-agent-gopls"
PLUGIN_DIR = os.path.join(CLAUDE_PLUGINS_DIR, PLUGIN_NAME, ".claude-plugin")
PLUGIN_FILE = "plugin.json"


@task(
    help={
        "targets": f"Comma separated list of targets to include. Possible values: all, {', '.join(build_tags[AgentFlavor.base].keys())}. Default: all",
        "flavor": f"Agent flavor to use. Possible values: {', '.join(AgentFlavor.__members__.keys())}. Default: {AgentFlavor.base.name}",
    }
)
def set_buildtags(
    _,
    targets="all",
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
):
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

    plugin = {
        "name": PLUGIN_NAME,
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
                        f"-tags={','.join(sorted(use_tags))}",
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
