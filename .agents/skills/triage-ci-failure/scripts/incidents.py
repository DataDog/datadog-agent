#!/usr/bin/env python3
"""Search Datadog incidents and render timelines, for CI-failure triage.

`pup incidents list` has no server-side "incident active at time T" filter and no way to
express the OR in "still open, or resolved after the failure" in one query. This script
owns that arithmetic: it turns a job-failure timestamp and a window into two bounded
`pup` queries, unions the results, and scores each incident against the failing job names
by parsing the auto-generated `[INTERNAL] <job> job is broken on the <branch> branch...`
title format.

`pup incidents get` also doesn't return the timeline despite its own help text advertising
`.data.timeline` — the real endpoint is `/api/v2/incidents/<id>/timeline` via `pup api`, and
it's mostly a Slack mirror (tens of KB per incident). `timeline` collapses that to the
handful of lines that matter: state transitions and human notes.

Every `pup` call is made with `--no-agent` so the output shape is the plain Datadog API
response regardless of who or what invokes this script.

Usage:
    incidents.py search --at <iso8601> [--job NAME]... [--window-hours 48] [--grace-hours 6]
                         [--service datadog-agent-ci | --no-service] [--text KEYWORD]
                         [--limit 100] [--json]
    incidents.py timeline <IR-nnnnn | numeric id | uuid> [--max-chars 400] [--json]

See /.agents/skills/triage-ci-failure/SKILL.md for how this fits into the wider triage flow.
"""

from __future__ import annotations

import argparse
import datetime as dt
import html
import json
import re
import subprocess
import sys
from dataclasses import dataclass, field
from enum import IntEnum
from functools import cached_property

# ============================== CONSTANTS ============================== #

INCIDENT_URL_FMT = "https://app.datadoghq.com/incidents/{public_id}"

# Auto-declared incident titles embed the job name verbatim, e.g.:
#   [INTERNAL] new-e2e-sbom: [--run &quot;TestSBOMKubeadmCrioSuite&quot;] job is broken on
#   the main branch of the `datadog-agent` repository
# Titles are HTML-escaped and the job name is *not* truncated at declaration time, but a
# future change to the (external) automation could start truncating it — see MatchTier.PREFIX.
TITLE_RE = re.compile(
    r"^\[INTERNAL\]\s+(?P<job>.+?)\s+job is broken on the (?P<branch>\S+) branch of the",
    re.IGNORECASE,
)

# Timeline `incident_status_change` cells carry field-level diffs as a generic `selections`
# list (see `_selection_values`). Most of them are noise for triage (team routing, slug,
# services) — only surface a diff if it touches one of these.
STATUS_FIELD_ALLOWLIST = {"severity", "state", "state_category", "root_cause", "summary"}


# ============================== MODEL ============================== #


class MatchTier(IntEnum):
    """How confidently an incident's title job maps onto a failing GitLab job.

    Ordered best-to-worst so that `min()` picks the strongest match and plain sorting puts
    the most relevant incidents first.
    """

    EXACT = 0  # identical job names
    BASE = 1  # identical once the parallel/matrix suffix is stripped
    PREFIX = 2  # one name is a prefix of the other (title truncation)
    TOKEN = 3  # share at least one distinctive token
    NONE = 4  # no relationship found

    def __str__(self) -> str:
        return self.name.lower()


def parse_iso(ts: str) -> float:
    """Parse an API timestamp to a POSIX timestamp.

    Datadog is inconsistent about the suffix — incident records use `Z`, timeline cells use
    `+00:00` — so comparisons have to go through real datetimes rather than string ordering.
    """
    return dt.datetime.fromisoformat(ts.replace("Z", "+00:00")).timestamp()


def _base_name(job_name: str) -> str:
    """Strip a parallel/matrix suffix: `foo: [shard 1/5]` -> `foo`."""
    return job_name.split(":", 1)[0].strip()


def _tokens(job_name: str) -> set[str]:
    return {t for t in re.split(r"[^a-zA-Z0-9]+", job_name.lower()) if len(t) >= 4}


def classify_pair(incident_job: str, failed_job: str) -> MatchTier:
    """Rate how strongly one incident-title job name corresponds to one failing job name."""
    if incident_job == failed_job:
        return MatchTier.EXACT
    if _base_name(incident_job) == _base_name(failed_job):
        return MatchTier.BASE
    if incident_job.startswith(failed_job) or failed_job.startswith(incident_job):
        return MatchTier.PREFIX
    if _tokens(incident_job) & _tokens(failed_job):
        return MatchTier.TOKEN
    return MatchTier.NONE


@dataclass
class Incident:
    """One Datadog incident, slimmed to the fields triage actually reads."""

    uuid: str
    public_id: int
    slug: str
    state: str
    severity: str
    created: str
    resolved: str | None
    title: str
    # Populated by `match_against`; empty until then, and empty means "matched nothing".
    matches: dict[str, MatchTier] = field(default_factory=dict)

    @classmethod
    def from_api(cls, raw: dict) -> Incident:
        """Build from one `data` element of a `pup incidents list` response."""
        attrs = raw["attributes"]
        return cls(
            uuid=raw["id"],
            public_id=attrs["public_id"],
            slug=attrs["fields"]["slug"]["value"],
            state=attrs["state"],
            severity=attrs["severity"],
            created=attrs["created"],
            resolved=attrs.get("resolved"),
            title=html.unescape(attrs.get("title") or ""),
        )

    @cached_property
    def _parsed_title(self) -> tuple[str | None, str | None]:
        match = TITLE_RE.match(html.unescape(self.title))
        if not match:
            return None, None
        return match.group("job").strip(), match.group("branch")

    @property
    def parsed_job(self) -> str | None:
        """The job name named in an auto-declared title, if this is one."""
        return self._parsed_title[0]

    @property
    def parsed_branch(self) -> str | None:
        """The branch named in an auto-declared title, if this is one."""
        return self._parsed_title[1]

    @property
    def url(self) -> str:
        return INCIDENT_URL_FMT.format(public_id=self.public_id)

    @property
    def best_tier(self) -> MatchTier:
        return min(self.matches.values(), default=MatchTier.NONE)

    def was_active_at(self, t_fail: float, window_s: float, grace_s: float) -> bool:
        """Was this incident plausibly active when the job failed?

        Incidents are declared *after* their first casualties, so the window is asymmetric:
        an incident created up to `grace_s` after the failure is still a candidate, and an
        incident is only ruled out by resolution if it resolved more than `window_s`
        *before* the failure.
        """
        if parse_iso(self.created) > t_fail + grace_s:
            return False
        if self.resolved is not None and parse_iso(self.resolved) < t_fail - window_s:
            return False
        return True

    def match_against(self, failed_jobs: list[str]) -> dict[str, MatchTier]:
        """Rate this incident against every failing job, keeping only the ones that match.

        An `[INTERNAL]` title names exactly one job, but that one job can legitimately
        correspond to several failing jobs at once — sibling shards of a parallel job all
        match its base name — so every match is reported rather than just the first found.
        """
        if not self.parsed_job:
            return {}
        rated = {job: classify_pair(self.parsed_job, job) for job in failed_jobs}
        return dict(rated.items())

    def _match_summary(self) -> str:
        tier = self.best_tier
        # `matches` retains jobs rated NONE, so "which jobs sit at the best tier" only names
        # real matches once NONE is ruled out — otherwise it would list the jobs that failed
        # to match and present them as though they had.
        if tier is MatchTier.NONE:
            return f"match={tier}"
        best = sorted(job for job, t in self.matches.items() if t == tier)
        if len(best) == 1:
            return f"match={tier} ({best[0]})"
        return f"match={tier} ({len(best)} jobs: {', '.join(best)})"

    def to_dict(self) -> dict:
        """Serialise for `--json`. Derived fields are included explicitly, since the
        properties backing them are invisible to generic dataclass serialisation."""
        return {
            "uuid": self.uuid,
            "public_id": self.public_id,
            "slug": self.slug,
            "state": self.state,
            "severity": self.severity,
            "created": self.created,
            "resolved": self.resolved,
            "title": self.title,
            "parsed_job": self.parsed_job,
            "parsed_branch": self.parsed_branch,
            "matches": {job: str(tier) for job, tier in self.matches.items()},
            "best_tier": str(self.best_tier),
            "url": self.url,
        }

    def __str__(self) -> str:
        """The three-line block `search` prints per incident."""
        resolved = self.resolved[:19] if self.resolved else "-"
        header = (
            f"{self.slug:<10} {self.state:<10} declared {self.created[:19]}  "
            f"resolved {resolved:<20} {self.severity:<6} {self._match_summary()}"
        )
        return f"{header}\n           {self.title}\n           {self.url}"


# ============================== PUP I/O ============================== #


class PupError(RuntimeError):
    """`pup` exited non-zero, or produced output that wasn't the JSON we expected."""


def run_pup(args: list[str]) -> dict:
    """Run `pup --no-agent <args>` and parse its stdout as JSON.

    `--no-agent` matters: without it `pup` wraps every response in a `{status, data,
    metadata}` envelope meant for agent consumption, which is not the shape this script
    (or a human running it outside an agent) expects.
    """
    proc = subprocess.run(
        ["pup", "--no-agent", *args],
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        raise PupError(proc.stderr.strip() or proc.stdout.strip() or f"pup exited {proc.returncode}")
    text = proc.stdout.strip()
    if not text:
        return {}
    try:
        return json.loads(text)
    except json.JSONDecodeError as exc:
        raise PupError(f"pup returned non-JSON output: {text[:200]!r}") from exc


def build_search_queries(window_start: int, window_end: int, service: str | None, text: str | None) -> tuple[str, str]:
    """The two time-bounded queries whose union covers "active when the job failed".

    The API can bound `created`/`resolved` server-side but cannot express "still open OR
    resolved after X" in a single query, so the disjunction is split across two calls:
    (a) incidents declared anywhere near the failure, and (b) older incidents still open.
    """
    prefix = ""
    if service:
        prefix += f"services:{service} "
    if text:
        prefix += f"{text} "
    declared_near_failure = f"{prefix}created_after:{window_start} created_before:{window_end}".strip()
    older_still_open = f"{prefix}state:(active OR stable) created_before:{window_start}".strip()
    return declared_near_failure, older_still_open


def fetch_incidents(query: str, limit: int) -> dict[str, dict]:
    """Run one incident query, returning raw `data` elements keyed by incident UUID."""
    resp = run_pup(["incidents", "list", "--query", query, "--limit", str(limit)])
    incidents = resp.get("data", {}).get("attributes", {}).get("incidents", [])
    return {item["data"]["id"]: item["data"] for item in incidents}


def fetch_timeline_cells(incident_ref: str) -> list[dict]:
    """Fetch an incident's timeline cells.

    The endpoint accepts a bare numeric public ID as well as a UUID, so `IR-` slugs only
    need their prefix stripped. It also defaults to 25 cells per page, which silently
    truncates most real incidents — hence the explicit page size.
    """
    numeric_id = incident_ref.strip()
    if numeric_id.upper().startswith("IR-"):
        numeric_id = numeric_id[3:]
    resp = run_pup(["api", f"/api/v2/incidents/{numeric_id}/timeline", "-F", "page[size]=200"])
    return resp.get("data", [])


# ============================== TIMELINE DIGEST ============================== #


def _selection_values(selections: list[dict]) -> dict[str, list]:
    return {sel["field"]["name"]: [c.get("value") for c in sel.get("choices", [])] for sel in selections}


def describe_status_change(cell: dict) -> str | None:
    """Render an `incident_status_change` cell, or None if it's routing/labelling noise."""
    content = cell["attributes"]["content"]
    if content.get("action") == "created":
        after = content.get("after") or {}
        return f"declared ({after.get('state', '?')}, {after.get('severity', '?')})"

    before = content.get("before") or {}
    after = content.get("after") or {}

    # Direct state transitions (active -> stable -> resolved/completed) carry `state`
    # directly, not via `selections`.
    if "state" in before or "state" in after:
        if before.get("state") != after.get("state"):
            return f"{before.get('state', '?')} \u2192 {after.get('state', '?')}"
        return None

    if "selections" in before or "selections" in after:
        before_vals = _selection_values(before.get("selections", []))
        after_vals = _selection_values(after.get("selections", []))
        changed = {k for k in set(before_vals) | set(after_vals) if before_vals.get(k) != after_vals.get(k)}
        changed &= STATUS_FIELD_ALLOWLIST
        if not changed:
            return None
        return "; ".join(
            f"{field_name}: {before_vals.get(field_name)} \u2192 {after_vals.get(field_name)}"
            for field_name in sorted(changed)
        )

    return None


def render_markdown_cell(cell: dict, max_chars: int) -> tuple[str, str]:
    """Flatten one Slack-mirrored note to a (author, single-line text) pair."""
    content = cell["attributes"]["content"]
    author = content.get("author") or {}
    who = author.get("name") or author.get("email") or "unknown"
    text = html.unescape(content.get("content") or "")
    text = " ".join(text.split())
    if len(text) > max_chars:
        text = text[:max_chars].rstrip() + "\u2026"
    return who, text


def slim_timeline(raw_cells: list[dict], max_chars: int) -> list[dict]:
    """Collapse the raw timeline (tens of KB, mostly Slack-mirror noise) to state
    transitions and human notes, chronologically ordered."""
    entries: list[dict] = []
    for cell in raw_cells:
        attrs = cell["attributes"]
        ts = attrs.get("created") or attrs.get("display_time")
        cell_type = attrs["cell_type"]
        if cell_type == "markdown":
            who, text = render_markdown_cell(cell, max_chars)
            if text:
                entries.append({"ts": ts, "kind": "note", "author": who, "text": text})
        elif cell_type == "incident_status_change":
            desc = describe_status_change(cell)
            if desc:
                entries.append({"ts": ts, "kind": "status", "text": desc})
        # incident_role_assignment_change / incident_integration_change /
        # incident_workstream_change: never useful for triage, dropped.
    entries.sort(key=lambda e: parse_iso(e["ts"]))
    return entries


# ============================== CLI ============================== #


def cmd_search(args: argparse.Namespace) -> int:
    """Find incidents that could explain a job failing at `--at`.

    Fetches the union of the two bounded queries, discards anything that wasn't actually
    active at the failure, then rates whatever survives against the failing job names.
    """
    t_fail = parse_iso(args.at)
    window_s = args.window_hours * 3600
    grace_s = args.grace_hours * 3600

    queries = build_search_queries(
        window_start=int(t_fail - window_s),
        window_end=int(t_fail + grace_s),
        service=None if args.no_service else args.service,
        text=args.text,
    )
    raw_by_id: dict[str, dict] = {}
    for query in queries:
        # Keyed by UUID, so the overlap between the two queries collapses on its own.
        raw_by_id.update(fetch_incidents(query, args.limit))

    incidents = [Incident.from_api(raw) for raw in raw_by_id.values()]
    incidents = [inc for inc in incidents if inc.was_active_at(t_fail, window_s, grace_s)]
    for inc in incidents:
        inc.matches = inc.match_against(args.job)

    # Strongest match first; ties broken by most recently declared. MatchTier is an IntEnum
    # ordered best-to-worst, so it sorts ascending, while recency has to be negated.
    incidents.sort(key=lambda inc: (inc.best_tier, -parse_iso(inc.created)))

    if args.json:
        print(json.dumps([inc.to_dict() for inc in incidents], indent=2))
    elif not incidents:
        print("No candidate incidents found in the window.")
    else:
        for inc in incidents:
            print(inc)
    return 0


def cmd_timeline(args: argparse.Namespace) -> int:
    """Render one incident's timeline as a compact, chronological digest."""
    entries = slim_timeline(fetch_timeline_cells(args.incident), args.max_chars)

    if args.json:
        print(json.dumps(entries, indent=2))
    elif not entries:
        print("No timeline entries.")
    else:
        for entry in entries:
            ts = entry["ts"][:19]
            if entry["kind"] == "status":
                print(f"{ts}  \u25cf  {entry['text']}")
            else:
                print(f"{ts}  {entry['author']:<20} {entry['text']}")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)

    search = sub.add_parser("search", help="Find incidents that may explain a CI job failure")
    search.add_argument("--at", required=True, help="ISO8601 timestamp the job failed at")
    search.add_argument(
        "--job", action="append", default=[], help="GitLab job name to match against incident titles (repeatable)"
    )
    search.add_argument(
        "--window-hours",
        type=float,
        default=48.0,
        help="How far back an incident may have resolved and still count (default: 48)",
    )
    search.add_argument(
        "--grace-hours",
        type=float,
        default=6.0,
        help="How long after the failure an incident may still have been declared (default: 6)",
    )
    search.add_argument(
        "--service", default="datadog-agent-ci", help="Datadog incident service filter (default: datadog-agent-ci)"
    )
    search.add_argument("--no-service", action="store_true", help="Drop the service filter (tier-3 broad search)")
    search.add_argument("--text", default=None, help="Extra free-text filter, e.g. a keyword pulled from the job log")
    search.add_argument("--limit", type=int, default=100, help="Max incidents per underlying query (default: 100)")
    search.add_argument("--json", action="store_true")
    search.set_defaults(func=cmd_search)

    timeline = sub.add_parser("timeline", help="Render an incident's timeline as a compact, chronological digest")
    timeline.add_argument("incident", help="Incident slug (IR-nnnnn), numeric public ID, or UUID")
    timeline.add_argument(
        "--max-chars", type=int, default=400, help="Truncate each note to this many characters (default: 400)"
    )
    timeline.add_argument("--json", action="store_true")
    timeline.set_defaults(func=cmd_timeline)

    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        return args.func(args)
    except PupError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
