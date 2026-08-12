"""Read the DataDog agent-payload version from go.mod.

Can be used as a library or run directly to write the Starlark constant file:

    python get_payload_version.py <go_mod_path> <output_bzl_path>
"""

import re
import sys


def get_payload_version(go_mod_path="go.mod"):
    """Return the agent-payload version (e.g. 'v5.0.198') from go.mod."""
    with open(go_mod_path) as f:
        for rawline in f:
            line = rawline.strip()
            whitespace_split = line.split(" ")
            if len(whitespace_split) < 2:
                continue
            if whitespace_split[0] != "github.com/DataDog/agent-payload/v5":
                continue
            # e.g. "github.com/DataDog/agent-payload/v5 v5.0.2"
            # or   "github.com/DataDog/agent-payload/v5 v5.0.1-0.20200826134834-1ddcfb686e3f"
            version_split = re.split(r'[ +]', line)
            if len(version_split) < 2:
                raise Exception("Versioning of agent-payload in go.mod has changed; update the version logic")
            version = version_split[1].split("-")[0].strip()
            if not re.search(r"^v\d+(\.\d+){2}$", version):
                raise Exception(f"Version of agent-payload in go.mod is invalid: '{version}'")
            return version

    raise Exception("Could not find valid version for agent-payload in go.mod")


if __name__ == "__main__":
    if len(sys.argv) < 3:
        print(f"Usage: {sys.argv[0]} <go_mod_path> <output_bzl_path>", file=sys.stderr)
        sys.exit(1)

    go_mod_path = sys.argv[1]
    output_path = sys.argv[2]

    version = get_payload_version(go_mod_path)
    with open(output_path, "w") as out:
        out.write(f'AGENT_PAYLOAD_VERSION = "{version}"\n')
