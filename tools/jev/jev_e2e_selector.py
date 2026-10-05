#!/usr/bin/env python3
"""Jev-based E2E test selector.

Decides, for each E2E test of a given suite (or a single test), whether it
should be executed on the current PR, by asking a Jev (TypeSafe System One)
model through the AI Gateway:

    state  = PR title + description + changed files + owning team + test code
         (PR number, changed files with modification kinds, merge base, author
          and impacted targets come from the DDCI Metadata Service when
          $DDCI_REQUEST_ID is set, else git merge-base + GitHub API fallback)
    questions:
      - should_execute (noul)   : run the test on this PR?
      - relation (choice)       : why (direct code, shared infra, packaging/CI, unrelated)
      - confidence (score)      : how confident in the decision

Outputs go-test compatible --run/--skip patterns (same integration point as the
coverage-based `--impacted` selection in tasks/new_e2e_tests.py), plus a JSON
artifact with the full decisions.

Usage (from repo root):
    GITHUB_TOKEN=... AI_GATEWAY_TOKEN=... python3 tools/jev/jev_e2e_selector.py \
        --suite fleet [--test TestFleetConfig] [--base main]

Token acquisition:
    - CI:      download authanywhere and pass --token-cmd 'authanywhere --audience rapid-ai-platform --raw --dc us1.ddbuild.io'
    - laptop:  ddtool auth token rapid-ai-platform --datacenter us1.staging.dog --raw (default)
Jev docs: https://datadoghq.atlassian.net/wiki/spaces/AIP/pages/7265386822
"""

from __future__ import annotations  # python 3.9 compat

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request

REPO = "DataDog/datadog-agent"
GITHUB_API = f"https://api.github.com/repos/{REPO}"
SYSTEMONE_PATH = "/v1/systemone"
# https://datadoghq.atlassian.net/wiki/spaces/DEVX/pages/5423334716/DDCI+Metadata+Service
DDCI_METADATA_URL = "https://cimetadataserver.us1.ddbuild.io/internal/ddci/metadata"

# Keep the state well below Jev's 32k tokens per question cap.
MAX_TEST_CODE_BYTES = 24_000
MAX_DESCRIPTION_BYTES = 4_000
MAX_CHANGED_FILES = 300

QUESTIONS = {
    "should_execute": {
        "type": "noul",
        "instructions": (
            "Given this PR's changed files and stated intent, could this PR "
            "plausibly affect what this e2e test verifies? 1.0 means the test "
            "should be executed, 0.0 means it is safe to skip."
        ),
    },
    "relation": {
        "type": "choice",
        "instructions": "What best describes the relationship between this PR and this e2e test?",
        "criteria": {
            "code_under_test": "The PR modifies code or behavior this test directly exercises",
            "shared_infra": "The PR modifies shared infrastructure, framework or configuration the test depends on",
            "packaging_ci": "The PR modifies build, packaging, dependencies or CI config affecting the test environment",
            "unrelated": "No plausible relationship between this PR and this test",
        },
    },
    "confidence": {
        "type": "score",
        "instructions": "How confident are you in your decision?",
        "criteria": [
            "Unsure, run the test to be safe",
            "Moderately confident",
            "Highly confident, decision is clear from the inputs",
        ],
    },
}


def run_cmd(cmd: list[str], env: dict | None = None) -> str:
    result = subprocess.run(cmd, capture_output=True, text=True, env=env)
    if result.returncode != 0:
        raise RuntimeError(f"command {cmd} failed: {result.stderr.strip()}")
    return result.stdout.strip()


def git(*args: str) -> str:
    return run_cmd(["git", *args])


def current_branch() -> str:
    """Current branch name, robust to detached-HEAD checkouts (GitLab CI).

    In CI the clone is a detached HEAD, where `git rev-parse --abbrev-ref HEAD`
    returns "HEAD"; prefer the CI-provided branch name in that case.
    """
    for env in ("CI_COMMIT_BRANCH", "CI_COMMIT_REF_NAME"):
        if os.environ.get(env) and os.environ[env] != "HEAD":
            return os.environ[env]
    branch = git("rev-parse", "--abbrev-ref", "HEAD")
    return branch if branch != "HEAD" else ""


def resolve_ref(base: str) -> str:
    """Resolve a branch name to a ref present in this clone.

    CI clones usually have no local `main` branch, only `origin/main`.
    """
    for candidate in (base, f"origin/{base}", f"refs/remotes/origin/{base}", f"refs/heads/{base}"):
        try:
            git("rev-parse", "--verify", "--quiet", candidate + "^{commit}")
            return candidate
        except RuntimeError:
            continue
    print(f"[info] ref '{base}' not found locally, fetching from origin")
    git("fetch", "origin", base)
    return f"origin/{base}"


def truncate(text: str, limit: int, label: str) -> str:
    if len(text.encode()) <= limit:
        return text
    out = text.encode()[:limit].decode(errors="ignore")
    return f"{out}\n[... truncated {label} ...]"


# ---------------------------------------------------------------- PR information


def fetch_ddci_metadata() -> dict | None:
    """PR/merge-base/changed-files metadata from the DDCI Metadata Service.

    Available in CI when the $DDCI_REQUEST_ID env var is set (change analysis
    for the PR). Returns None when unavailable (e.g. local run, no analysis)
    so callers can fall back to git + GitHub API.
    """
    request_id = os.environ.get("DDCI_REQUEST_ID")
    if not request_id:
        print("[info] DDCI_REQUEST_ID not set, skipping DDCI metadata")
        return None
    try:
        with urllib.request.urlopen(f"{DDCI_METADATA_URL}/{request_id}", timeout=15) as resp:
            data = json.load(resp)
    except Exception as e:
        print(f"[warn] could not fetch DDCI metadata for request {request_id}: {e}")
        return None
    event = data.get("event", {})
    if event.get("status") != "completed":
        print(f"[warn] DDCI analysis status is '{event.get('status')}', not using it")
        return None
    req = event.get("request", {})
    results = event.get("results") or {}
    meta = {
        "request_id": request_id,
        "base_commit": req.get("base_commit"),
        "head_commit": req.get("head_commit"),
        "base_ref": req.get("base_ref"),
        "ref": req.get("ref"),
        "pr_number": (req.get("pull_request") or {}).get("number"),
        "author": (req.get("user_info") or {}).get("github_handle"),
        "changed_files": [
            (f["path"], f.get("kind", "").replace("FILE_MODIFICATION_KIND_", "").lower())
            for f in results.get("changed_files", [])
        ],
        "impacted_targets": [t["name"] for t in results.get("targets", [])],
    }
    print(
        f"[info] DDCI metadata: PR #{meta['pr_number']}, merge base {str(meta['base_commit'])[:8]}, "
        f"{len(meta['changed_files'])} changed files, {len(meta['impacted_targets'])} impacted targets"
    )
    return meta


def fetch_pr_info(base: str, ddci: dict | None) -> dict:
    """PR title/description from the GitHub API (uses GITHUB_TOKEN if present)."""
    branch = current_branch()
    number = ddci.get("pr_number") if ddci else None
    token = os.environ.get("GITHUB_TOKEN")
    pr: dict = {"branch": branch, "title": "", "description": ""}
    if ddci:
        pr["author"] = ddci.get("author")
    if not token:
        print("[warn] GITHUB_TOKEN not set, cannot fetch PR description")
        return pr

    if number:
        req = urllib.request.Request(
            f"{GITHUB_API}/pulls/{number}",
            headers={"Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json"},
        )
    elif branch:
        print(f"[info] no DDCI PR number, looking up PR by branch {branch}")
        req = urllib.request.Request(
            f"{GITHUB_API}/pulls?head={REPO.split(':')[0]}:{urllib.parse.quote(branch)}&state=open&per_page=1",
            headers={"Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json"},
        )
    else:
        print("[warn] cannot determine branch name (detached HEAD, no CI env), cannot look up PR")
        return pr
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            payload = json.load(resp)
    except Exception as e:
        print(f"[warn] GitHub API request failed: {e}")
        return pr
    if isinstance(payload, list):
        payload = payload[0] if payload else {}
        if not payload:
            print(f"[warn] no open PR found for branch {branch}")
            return pr
    print(f"[info] PR #{payload.get('number')}: {payload.get('title')}")
    pr.update(
        {"number": payload.get("number"), "title": payload.get("title", ""), "description": payload.get("body") or ""}
    )
    return pr


def changed_files(base: str, ddci: dict | None) -> tuple[list, str]:
    """Changed (path, modification_kind) pairs and the merge base used.

    Prefers the DDCI metadata (GitHub-computed merge base + changed files with
    modification kinds), falls back to a local git merge-base diff.
    """
    if ddci and ddci.get("changed_files") is not None:
        files = ddci["changed_files"][:MAX_CHANGED_FILES]
        return files, ddci.get("base_commit") or base
    ref = resolve_ref(base)
    try:
        merge_base = git("merge-base", "HEAD", ref)
    except RuntimeError:
        merge_base = ref
    files = [(f, "") for f in git("diff", "--name-only", merge_base, "HEAD").splitlines()]
    return files[:MAX_CHANGED_FILES], merge_base


# ---------------------------------------------------------------- test discovery


def extract_function(code: str, func_header: str) -> str:
    """Extract a Go function body from `code`, from the header line to balanced braces."""
    lines = code.splitlines(keepends=True)
    start = None
    for i, line in enumerate(lines):
        if line.strip().startswith(func_header):
            start = i
            break
    if start is None:
        return func_header
    out, depth, opened = [], 0, False
    for line in lines[start:]:
        out.append(line)
        depth += line.count("{") - line.count("}")
        if "{" in line:
            opened = True
        if opened and depth <= 0:
            break
    return "".join(out)


def find_test_code(suite_dir: str, test_name: str) -> tuple[str, str]:
    """Return (file_path, source) for a test entry point or suite method."""
    for root, _, go_files in os.walk(suite_dir):
        for f in sorted(go_files):
            if not f.endswith(".go") or f.endswith("_test_helpers.go"):
                continue
            path = os.path.join(root, f)
            try:
                code = open(path, encoding="utf-8", errors="ignore").read()
            except OSError:
                continue
            for pattern in (rf"func {test_name}\(", rf"func \(s \*\w+\) {test_name}\("):
                m = re.search(pattern, code)
                if m:
                    header = m.group(0)
                    return path, extract_function(code, header)
    return "", test_name  # not found: fall back to the bare name


def list_suites(suite_dir: str) -> list:
    """All test entry points in the suite dir, with the full source of their file.

    Each e2e suite file defines one `func TestXxx(t *testing.T)` entry point that
    runs a suite of `func (s *...) TestYyy()` methods, so evaluating per entry
    point with the whole file gives Jev the complete test logic.
    Returns a list of (entry_point, file_path, file_code).
    """
    suites = []
    for root, _, go_files in os.walk(suite_dir):
        for f in sorted(go_files):
            if not f.endswith(".go"):
                continue
            path = os.path.join(root, f)
            code = open(path, encoding="utf-8", errors="ignore").read()
            entries = re.findall(r"^func (Test\w+)\(", code, re.MULTILINE)
            for entry in entries:
                suites.append((entry, path, code))
    return sorted(suites)


# ---------------------------------------------------------------- Jev client


def get_ai_gateway_token(args: argparse.Namespace) -> str:
    if args.token:
        return args.token
    if os.environ.get("AI_GATEWAY_TOKEN"):
        return os.environ["AI_GATEWAY_TOKEN"]
    if args.token_cmd:
        return run_cmd(args.token_cmd.split()).strip()
    # laptop fallback
    return run_cmd(["ddtool", "auth", "token", "rapid-ai-platform", "--datacenter", "us1.staging.dog", "--raw"]).strip()


def ask_jev(args: argparse.Namespace, token: str, state: str) -> dict:
    payload = {
        "state": state,
        "model": args.model,
        "questions": QUESTIONS,
    }
    req = urllib.request.Request(
        f"https://ai-gateway.{args.dc}{SYSTEMONE_PATH}",
        data=json.dumps(payload).encode(),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
            "source": args.source,
            "org-id": "2",
            "x-dd-tag-ddagent-ci": "innovation-week-experiment",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            return json.load(resp)
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="ignore")
        raise RuntimeError(f"AI Gateway HTTP {e.code}: {body}") from e


# ---------------------------------------------------------------- main


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--suite", required=True, help="E2E suite under test/new-e2e/tests/ (e.g. fleet)")
    parser.add_argument("--test", help="Restrict to a single test entry point or suite method")
    parser.add_argument(
        "--base", default=os.environ.get("COMPARE_TO_BRANCH", "main"), help="Base branch to diff against"
    )
    parser.add_argument("--team", default=None, help="Owning team (defaults to the suite directory name)")
    parser.add_argument("--model", default="datadoginternal/openjev-medium")
    parser.add_argument("--dc", default="us1.ddbuild.io", help="AI Gateway datacenter")
    parser.add_argument("--source", default="datadog-agent", help="source header for AI Gateway")
    parser.add_argument("--token", help="Raw internal auth token (default: $AI_GATEWAY_TOKEN)")
    parser.add_argument("--token-cmd", help="Command producing a raw token, e.g. authanywhere invocation")
    parser.add_argument(
        "--run-threshold", type=float, default=0.5, help="should_execute value above which the test runs"
    )
    parser.add_argument("--output", default="jev_e2e_decisions.json", help="JSON output path")
    parser.add_argument(
        "--dry-run", action="store_true", help="Print the state that would be sent, without calling Jev"
    )
    args = parser.parse_args()

    suite_dir = os.path.join("test", "new-e2e", "tests", args.suite)
    if not os.path.isdir(suite_dir):
        print(f"error: no e2e suite at {suite_dir}", file=sys.stderr)
        return 1
    team = args.team or args.suite

    print(f"[info] suite={args.suite} team={team} base={args.base}")
    ddci = fetch_ddci_metadata()
    pr = fetch_pr_info(args.base, ddci)
    files, merge_base = changed_files(args.base, ddci)
    print(f"[info] {len(files)} changed files (merge base {str(merge_base)[:8]})")

    suites = list_suites(suite_dir)
    if args.test:
        # --test matches an entry point, or a suite method: use the entry point of its file
        suites = [s for s in suites if s[0] == args.test]
        if not suites:
            for root, _, go_files in os.walk(suite_dir):
                for f in sorted(go_files):
                    if not f.endswith(".go"):
                        continue
                    path = os.path.join(root, f)
                    code = open(path, encoding="utf-8", errors="ignore").read()
                    if re.search(rf"^func \(s \*\w+\) {re.escape(args.test)}\(", code, re.MULTILINE):
                        entries = re.findall(r"^func (Test\w+)\(", code, re.MULTILINE)
                        suites = [s for s in list_suites(suite_dir) if s[0] in entries]
        if not suites:
            print(f"error: test {args.test} not found in {suite_dir}", file=sys.stderr)
            return 1
    print(f"[info] evaluating {len(suites)} tests: {', '.join(s[0] for s in suites)}")

    token = None if args.dry_run else get_ai_gateway_token(args)

    decisions = []
    for name, path, code in suites:
        files_section = "\n".join(f"- {f} ({kind})" if kind else f"- {f}" for f, kind in files)
        author = f", author: @{pr['author']}" if pr.get("author") else ""
        impacted = ""
        if ddci and ddci.get("impacted_targets"):
            impacted = "\n\n## Impacted build targets (from DDCI build impact analysis)\n" + ", ".join(
                ddci["impacted_targets"][:100]
            )
        state = (
            "## PR under review\n"
            f"Title: {pr.get('title') or '(unknown)'}{author}\n"
            f"Description:\n{truncate(pr.get('description') or '(none)', MAX_DESCRIPTION_BYTES, 'description')}\n"
            f"Owning team of the E2E suite: {team}\n\n"
            f"## Files changed in this PR (merge base {str(merge_base)[:12]}, {len(files)} files)\n"
            f"{files_section}{impacted}\n\n"
            "## E2E test under evaluation\n"
            f"Test: {name}\n"
            f"Suite: {args.suite} ({path})\n"
            f"Code:\n```go\n{truncate(code, MAX_TEST_CODE_BYTES, 'test code')}\n```\n\n"
            "Context: this is a test in the datadog-agent repository, a large Go monorepo. "
            "E2E tests provision real VMs and are expensive to run. Decide whether this PR "
            "plausibly affects what this test verifies."
        )
        if args.dry_run:
            print(f"--- state for {name} (dry run, not sent) ---\n{state}\n")
            decisions.append({"test": name, "dry_run": True})
            continue

        # Print exactly what is sent to Jev for every call
        print(f"\n--- Jev call for {name} ---")
        print(f"endpoint: https://ai-gateway.{args.dc}{SYSTEMONE_PATH}  model: {args.model}  source: {args.source}")
        print(f"state ({len(state)} chars):\n{state}")
        print(f"questions: {json.dumps(QUESTIONS)}")
        print("--- end of Jev call input ---")

        try:
            answer = ask_jev(args, token, state)
        except Exception as e:  # fail open: if Jev is unavailable, run the test
            print(f"[warn] Jev call failed for {name}: {e} -> defaulting to RUN")
            decisions.append({"test": name, "should_execute": 1.0, "error": str(e), "decision": "run"})
            continue

        a = answer["answers"]
        should = a["should_execute"]["noul"]
        relation = a["relation"]["choice"]
        confidence = a["confidence"]["score"]
        decision = "run" if should >= args.run_threshold or relation != "unrelated" else "skip"
        row = {
            "test": name,
            "should_execute": should,
            "relation": relation,
            "confidence": confidence,
            "decision": decision,
            "usage": answer.get("usage"),
        }
        decisions.append(row)
        print(
            f"[jev] {name:<45} -> {decision.upper():4}  should_execute={should:.2f}  "
            f"relation={relation}  confidence={confidence:.2f}"
        )

    if not args.dry_run:
        to_run = sorted({d["test"] for d in decisions if d["decision"] == "run"})
        to_skip = sorted({d["test"] for d in decisions if d["decision"] == "skip"})
        summary = {
            "suite": args.suite,
            "team": team,
            "base": args.base,
            "pr": pr.get("number"),
            "changed_files": files,
            "run": to_run,
            "skip": to_skip,
            "decisions": decisions,
        }
        with open(args.output, "w") as f:
            json.dump(summary, f, indent=2)
        print(f"\n[summary] run: {to_run or 'none'}")
        print(f"[summary] skip: {to_skip or 'none'}")
        print(
            f"[summary] go test flag: --skip '{'|'.join(to_skip)}'" if to_skip else "[summary] go test flag: (run all)"
        )
        print(f"[summary] decisions written to {args.output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
