"""Bridges patch_ng's in-place semantics to Bazel's "one action -> declared output" model."""

import argparse
import shutil
import sys
from pathlib import Path

import patch_ng

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("src", type=Path)
parser.add_argument("out", type=Path)
parser.add_argument("patches", type=Path, nargs="+")
parser.add_argument("-p", "--strip", type=int)
args = parser.parse_args()

patch_ng.logger.addHandler(patch_ng.streamhandler)
shutil.copyfile(args.src, args.out)
for patch in args.patches:
    if not (patchset := patch_ng.fromfile(patch)):
        sys.exit(f"cannot parse: {patch}")
    if not patchset.apply(strip=args.strip, root=args.out.parent):
        sys.exit(f"patch failed: {patch}")
