"""Load candidate YAML specs from `.coordinator/candidates/` into db.yaml.

Each `*.yaml` file under the candidates directory is parsed and inserted
into db.candidates (keyed by `id`). Idempotent: existing candidates are
left untouched unless `--overwrite` is passed.

Usage:
  PYTHONPATH=tasks python -m coordinator.seed_candidates
  PYTHONPATH=tasks python -m coordinator.seed_candidates --overwrite
"""

from __future__ import annotations

import argparse
import datetime as _dt
import sys
from pathlib import Path

import yaml

from .db import load_db, save_db, state_dir
from .schema import Candidate, CandidateStatus, Phase


CANDIDATES_DIR = "candidates"


def _load_one(path: Path) -> Candidate:
    with path.open() as f:
        data = yaml.safe_load(f)
    return Candidate(
        id=data["id"],
        description=str(data["description"]).strip(),
        source=data.get("source", "seed"),
        target_components=list(data.get("target_components", [])),
        phase=Phase(str(data["phase"])),
        status=CandidateStatus(data.get("status", "proposed")),
        proposed_at=data.get("proposed_at") or _dt.datetime.now().isoformat(timespec="seconds"),
        approach_family=data.get("approach_family", "unspecified"),
        parent_candidates=list(data.get("parent_candidates", [])),
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="seed_candidates")
    parser.add_argument("--root", default=".")
    parser.add_argument(
        "--overwrite",
        action="store_true",
        help="Replace existing candidates with the YAML version.",
    )
    args = parser.parse_args(argv)

    root = Path(args.root)
    candidates_dir = state_dir(root) / CANDIDATES_DIR
    if not candidates_dir.is_dir():
        print(f"no candidates dir at {candidates_dir}", file=sys.stderr)
        return 1

    db = load_db(root)
    added, skipped, replaced = 0, 0, 0
    for yml in sorted(candidates_dir.glob("*.yaml")):
        cand = _load_one(yml)
        if cand.id in db.candidates:
            if args.overwrite:
                db.candidates[cand.id] = cand
                replaced += 1
                print(f"replaced {cand.id}  ({yml.name})")
            else:
                skipped += 1
                print(f"skipped {cand.id} (already present)  ({yml.name})")
            continue
        db.candidates[cand.id] = cand
        added += 1
        print(f"added {cand.id}  ({yml.name})")

    save_db(db, root)
    print(f"\ntotal: {added} added, {replaced} replaced, {skipped} skipped")
    return 0


if __name__ == "__main__":
    sys.exit(main())
