import argparse
import subprocess
import sys

parser = argparse.ArgumentParser()
parser.add_argument("--stdin")
parser.add_argument("--stdout")
parser.add_argument("--stderr")
parser.add_argument("cmd", nargs=argparse.REMAINDER)
args = parser.parse_args()
sys.exit(subprocess.call(
    args.cmd[args.cmd[:1] == ["--"]:],
    stdin=open(args.stdin) if args.stdin else None,
    stdout=open(args.stdout, "w") if args.stdout else None,
    stderr=open(args.stderr, "w") if args.stderr else None,
))
