"""Import existing `eval-results/<component>/` reports into db.validations.

One-off utility for ingesting out-of-band eval-component runs (e.g. the
bocpd / scanmw / scanwelch workspace runs completed before the
workspace_validate pipeline was wired). Each becomes a PendingValidation
with status=done and no experiment_id (they aren't tied to a specific
coordinator experiment).

Usage:
  PYTHONPATH=tasks python -m coordinator.import_validations
"""

from __future__ import annotations

import argparse
import datetime as _dt
import json
import sys
import uuid
from pathlib import Path

from .db import load_db, save_db
from .schema import PendingValidation


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="import_validations")
    parser.add_argument("--root", default=".")
    parser.add_argument("--eval-dir", default="eval-results")
    args = parser.parse_args(argv)

    root = Path(args.root)
    eval_dir = root / args.eval_dir
    if not eval_dir.is_dir():
        print(f"no eval-results dir at {eval_dir}", file=sys.stderr)
        return 1

    db = load_db(root)
    imported = 0
    for sub in sorted(eval_dir.iterdir()):
        if not sub.is_dir():
            continue
        report = sub / "report.json"
        if not report.exists():
            continue
        # Skip dirs that are already tracked
        if any(
            v.local_path and Path(v.local_path).resolve() == sub.resolve()
            for v in db.validations.values()
        ):
            print(f"skip {sub.name} (already tracked)")
            continue
        with report.open() as f:
            r = json.load(f)
        component = r.get("component", sub.name)
        reco = r.get("recommendation")
        delta_max = (r.get("summary") or {}).get("delta_max")
        vid = f"val-manual-{uuid.uuid4().hex[:8]}"
        db.validations[vid] = PendingValidation(
            id=vid,
            experiment_id="manual",
            candidate_id=f"manual-{component}",
            detector=component,
            workspace=f"workspace-evals-{component}",
            remote_output_dir="(manual import)",
            dispatched_at="(manual import)",
            status="done",
            completed_at=_dt.datetime.now().isoformat(timespec="seconds"),
            local_path=str(sub),
            recommendation=reco,
            delta_max=delta_max,
        )
        imported += 1
        print(f"imported {sub.name}: reco={reco}, delta_max={delta_max}")

    save_db(db, root)
    print(f"\nimported {imported} validations into db.yaml")
    return 0


if __name__ == "__main__":
    sys.exit(main())
