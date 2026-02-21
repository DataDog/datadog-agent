"""Transform release.json into release.bzl.

This allows us to pull the release config variables (such as window driver versions) from there.
"""

import json
import sys

from python.runfiles import runfiles


def main(args):
    """Usgae: main input_path output_path."""
    file_path = runfiles.Create().Rlocation("_main/release.json")
    with open(args[1]) as inp:
        release_info = json.loads(inp.read())
        with open(args[2], 'w') as out:
            out.write('"""Release constants.  Generated from release.json. Do not edit."""\n')
            out.write('base_branch = "%s"\n' % release_info["base_branch"])
            out.write('current_milestone = "%s"\n' % release_info["current_milestone"])
            out.write('dependencies = %s\n' % json.dumps(release_info["dependencies"], indent=4))
            out.write('last_stable = %s\n' % json.dumps(release_info["last_stable"], indent=4))


if __name__ == "__main__":
    main(sys.argv)
