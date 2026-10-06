"""Git / GitHub / DDCI context for the Jev e2e tooling."""

from __future__ import annotations

import json
import os
import urllib.request

from invoke import Context

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.utils import join_command

DDCI_METADATA_URL = "https://cimetadataserver.us1.ddbuild.io/internal/ddci/metadata"

# Keep the state well below Jev's 32k tokens per question cap.


MAX_CHANGED_FILES = 300
# Jev caps each question at ~32k tokens (~120KB of text); keep the whole
# state well below that.


MAX_DESCRIPTION_BYTES = 4_000


def run_cmd(cmd: list[str]) -> str:
    """Run a command through invoke - the canonical task mechanism (see the
    ~140 ctx.run call sites) - with the Windows-safe join_command quoting.

    Raises RuntimeError with the command's stderr on failure.
    """
    result = Context().run(join_command(cmd), hide=True, warn=True, encoding="utf-8", timeout=60)
    if result.failed:
        raise RuntimeError(f"command {join_command(cmd)} failed: {result.stderr.strip()}")
    return result.stdout.strip()


def git(*args: str) -> str:
    return run_cmd(["git", *args])


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
    """PR title/description via the shared GithubAPI (uses GITHUB_TOKEN if present)."""
    # Current branch, robust to the detached-HEAD CI checkouts where rev-parse
    # returns "HEAD": prefer the CI-provided branch name in that case
    branch = ""
    for env in ("CI_COMMIT_BRANCH", "CI_COMMIT_REF_NAME"):
        if os.environ.get(env) and os.environ[env] != "HEAD":
            branch = os.environ[env]
            break
    if not branch:
        branch = git("rev-parse", "--abbrev-ref", "HEAD")
    branch = "" if branch == "HEAD" else branch
    number = ddci.get("pr_number") if ddci else None
    pr: dict = {"branch": branch, "title": "", "description": ""}
    if ddci:
        pr["author"] = ddci.get("author")
    if not os.environ.get("GITHUB_TOKEN"):
        print("[warn] GITHUB_TOKEN not set, cannot fetch PR description")
        return pr

    github = GithubAPI()
    try:
        if number:
            pull = github.get_pr(int(number))
        elif branch:
            print(f"[info] no DDCI PR number, looking up PR by branch {branch}")
            pulls = list(github.get_pr_for_branch(head_branch_name=branch))
            pull = pulls[0] if pulls else None
        else:
            print("[warn] cannot determine branch name (detached HEAD, no CI env), cannot look up PR")
            return pr
        if pull is None:
            print(f"[warn] no open PR found for branch {branch}")
            return pr
        print(f"[info] PR #{pull.number}: {pull.title}")
        pr.update({"number": pull.number, "title": pull.title, "description": pull.body or ""})
    except Exception as e:
        print(f"[warn] GitHub API request failed: {e}")
    return pr


def changed_files(base: str, ddci: dict | None) -> tuple[list, str]:
    """Changed (path, modification_kind) pairs and the merge base used.

    Prefers the DDCI metadata (GitHub-computed merge base + changed files with
    modification kinds), falls back to a local git merge-base diff.
    """
    if ddci and ddci.get("changed_files") is not None:
        files = ddci["changed_files"][:MAX_CHANGED_FILES]
        return files, ddci.get("base_commit") or base
    # Resolve the base branch to a ref present in this clone (CI clones have
    # no local main, only origin/main); fetch from origin as a last resort
    ref = ""
    for candidate in (base, f"origin/{base}", f"refs/remotes/origin/{base}", f"refs/heads/{base}"):
        try:
            git("rev-parse", "--verify", "--quiet", candidate + "^{commit}")
            ref = candidate
            break
        except RuntimeError:
            continue
    if not ref:
        print(f"[info] ref '{base}' not found locally, fetching from origin")
        git("fetch", "origin", base)
        ref = "FETCH_HEAD"
    try:
        merge_base = git("merge-base", "HEAD", ref)
    except RuntimeError:
        merge_base = ref
    files = [(f, "") for f in git("diff", "--name-only", merge_base, "HEAD").splitlines()]
    return files[:MAX_CHANGED_FILES], merge_base
