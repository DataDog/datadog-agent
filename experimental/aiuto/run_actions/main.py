"""run_actions: run several shell commands and present their output in sections."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

# This the py portable app thing seems very broken or I am not
# using it right. This is needed to get imports to resolve.
prog_root = str(Path(sys.argv[0]).parent.parent)
sys.path.append(prog_root)

# for path in sys.path:
#    print(" path", path)

from run_actions import action as action_lib
from run_actions import render, runner

_DESCRIPTION = """\
Run several actions (single bash command lines), in parallel by default, and
print each action's captured stdout and stderr as a section once all actions
have finished. Exits 0 if every action exits 0, otherwise 1.
"""

_EPILOG = """\
action forms:
  name='command line with args'   named action
  'command line with args'        the name is the command itself

  A leading WORD= is always read as a name, so 'FOO=1 make' is the action
  named FOO running '1 make'. To set an environment variable, give the action
  a name: name='FOO=1 make'.

boolean flags (Bazel style):
  --serial[=true|false], --noserial
      Run actions one at a time, in command line order (default: false).
  --show_errors_last[=true|false], --noshow_errors_last
      Show passing actions first, then failing ones (default: true).
"""

_TRUE_VALUES = ("true", "1", "yes")
_FALSE_VALUES = ("false", "0", "no")


def parse_bool(value: str) -> bool:
    lowered = value.lower()
    if lowered in _TRUE_VALUES:
        return True
    if lowered in _FALSE_VALUES:
        return False
    raise argparse.ArgumentTypeError("expected true or false, got %r" % value)


def _normalize_bool_flags(argv: list, names: list) -> list:
    """Rewrites Bazel-style bool flags into --name=value form.

    --name becomes --name=true and --noname becomes --name=false. A bare
    --name never consumes the next argument, which is always an action.
    """
    result = []
    for arg in argv:
        rewritten = arg
        for name in names:
            if arg == "--" + name:
                rewritten = "--" + name + "=true"
            elif arg == "--no" + name:
                rewritten = "--" + name + "=false"
        result.append(rewritten)
    return result


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="run_actions",
        usage="%(prog)s [-h] [--[no]serial] [--[no]show_errors_last] action [action ...]",
        description=_DESCRIPTION,
        epilog=_EPILOG,
        formatter_class=argparse.RawDescriptionHelpFormatter,
        allow_abbrev=False,
    )
    parser.add_argument(
        "--serial",
        type=parse_bool,
        default=False,
        help=argparse.SUPPRESS,  # Described in the epilog; value only via '='.
    )
    parser.add_argument(
        "--show_errors_last",
        type=parse_bool,
        default=True,
        help=argparse.SUPPRESS,  # Described in the epilog; value only via '='.
    )
    parser.add_argument("actions", nargs="*", metavar="action", help="[name=]command")
    return parser


def parse_args(argv: list) -> argparse.Namespace:
    # Everything after '--' is an action. We split it off ourselves because
    # argparse's handling of '--' with intermixed args varies across versions.
    flag_args = argv
    trailing_actions = []
    if "--" in argv:
        split = argv.index("--")
        flag_args = argv[:split]
        trailing_actions = argv[split + 1 :]
    parser = build_parser()
    args = parser.parse_intermixed_args(_normalize_bool_flags(flag_args, ["show_errors_last", "serial"]))
    args.actions = args.actions + trailing_actions
    if len(args.actions) == 0:
        parser.error("at least one action is required")
    return args


def main(argv: list = None) -> int:
    if argv is None:
        argv = sys.argv[1:]
    args = parse_args(argv)
    actions = action_lib.parse_actions(args.actions)
    if args.serial:
        results = runner.run_serial(actions)
    else:
        results = runner.run_parallel(actions)
    sys.stdout.flush()
    render.render(results, args.show_errors_last, sys.stdout.buffer)
    for result in results:
        if not result.passed():
            return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
