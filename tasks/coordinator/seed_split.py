"""Seed the DataSplit into db.yaml.

Default split matches the plan §4 rev-7 discussion:
  train  (6): 059_fortnite, 093_cloudflare, 221_base, 353_postmark, 546_cloudflare, 703_shopify
  lockbox(4): food_delivery_redis, 213_pagerduty, 211_doordash, 063_twilio

`sealed_hash` is a SHA-256 over the sorted lockbox list, so any future
change to lockbox membership produces a different hash — visible in
metrics.md and recorded in db.yaml.

Usage:
  PYTHONPATH=tasks python -m coordinator.seed_split
  PYTHONPATH=tasks python -m coordinator.seed_split --train A,B,C --lockbox D,E
"""

from __future__ import annotations

import argparse
import hashlib
import sys
from pathlib import Path

from .db import load_db, save_db
from .schema import DataSplit


DEFAULT_TRAIN = [
    "059_fortnite",
    "093_cloudflare",
    "221_base",
    "353_postmark",
    "546_cloudflare",
    "703_shopify",
]
DEFAULT_LOCKBOX = [
    "food_delivery_redis",
    "213_pagerduty",
    "211_doordash",
    "063_twilio",
]


def compute_sealed_hash(lockbox: list[str]) -> str:
    key = ",".join(sorted(lockbox))
    return hashlib.sha256(key.encode()).hexdigest()


def make_split(train: list[str], lockbox: list[str]) -> DataSplit:
    return DataSplit(
        train=list(train),
        lockbox=list(lockbox),
        sealed_hash=compute_sealed_hash(lockbox),
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="seed_split")
    parser.add_argument("--root", default=".")
    parser.add_argument("--train", default=",".join(DEFAULT_TRAIN))
    parser.add_argument("--lockbox", default=",".join(DEFAULT_LOCKBOX))
    parser.add_argument(
        "--overwrite",
        action="store_true",
        help="Replace an existing split (changes sealed_hash; requires ack).",
    )
    args = parser.parse_args(argv)

    train = [s.strip() for s in args.train.split(",") if s.strip()]
    lockbox = [s.strip() for s in args.lockbox.split(",") if s.strip()]

    if set(train) & set(lockbox):
        print("error: train and lockbox overlap", file=sys.stderr)
        return 2

    db = load_db(Path(args.root))
    if db.split is not None and not args.overwrite:
        print(
            f"split already present (sealed_hash={db.split.sealed_hash[:10]}); "
            "pass --overwrite to replace.",
            file=sys.stderr,
        )
        return 1

    db.split = make_split(train, lockbox)
    save_db(db, Path(args.root))
    print(f"train  ({len(train)}):   {', '.join(train)}")
    print(f"lockbox({len(lockbox)}): {', '.join(lockbox)}")
    print(f"sealed_hash: {db.split.sealed_hash}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
