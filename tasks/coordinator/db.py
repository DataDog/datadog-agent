from __future__ import annotations

import os
import tempfile
from pathlib import Path

import yaml

from .config import CONFIG
from .schema import (
    Baseline,
    BudgetState,
    Db,
    Phase,
    PhaseState,
    db_to_dict,
    dict_to_db,
)

DEFAULT_STATE_DIR = Path(".coordinator")


def state_dir(root: Path = Path(".")) -> Path:
    return root / DEFAULT_STATE_DIR


def db_path(root: Path = Path(".")) -> Path:
    return state_dir(root) / "db.yaml"


def empty_db() -> Db:
    return Db(
        schema_version=1,
        baseline=None,
        experiments={},
        candidates={},
        phase_state=PhaseState(
            # Start at Phase.ONE — Phase.ZERO existed as a setup state but
            # every candidate the proposer generates is phase="1", and the
            # scheduler filters by phase. Having the default be ZERO meant
            # freshly-proposed candidates couldn't be picked.
            current_phase=Phase.ONE,
            best_score=0.0,
            plateau_counter=0,
            phase_start_iter=0,
        ),
        budget=BudgetState(
            wall_hours_used=0.0,
            wall_hours_ceiling=CONFIG.default_wall_hours_ceiling,
            api_tokens_used=0,
            api_token_ceiling=CONFIG.api_token_ceiling,
        ),
        iterations=[],
        # Empty on startup: every component gets one eval-component dispatch
        # on plateau of a shipping family targeting it. Historical reports
        # (bocpd/scanmw/scanwelch in eval-results/) validated the BASELINE
        # versions of those components — after the coordinator modifies
        # them, that historical data is stale. Re-run on plateau.
        components_eval_dispatched=[],
    )


def load_db(root: Path = Path(".")) -> Db:
    p = db_path(root)
    if not p.exists():
        return empty_db()
    with p.open() as f:
        d = yaml.safe_load(f) or {}
    if not d:
        return empty_db()
    return dict_to_db(d)


def save_db(db: Db, root: Path = Path(".")) -> None:
    """Atomic write: tmp + rename to avoid partial state on crash."""
    p = db_path(root)
    p.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=p.parent, prefix=".db-", suffix=".tmp")
    try:
        with os.fdopen(fd, "w") as f:
            yaml.safe_dump(db_to_dict(db), f, sort_keys=False)
        os.replace(tmp, p)
    except BaseException:
        if os.path.exists(tmp):
            os.unlink(tmp)
        raise


def set_baseline(db: Db, baseline: Baseline) -> Db:
    db.baseline = baseline
    return db
