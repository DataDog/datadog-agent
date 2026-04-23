from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path

from tools.logs_agent_ai.constants import INVARIANT_PAGE_PATHS, REVIEW_MODEL_ENV, DEFAULT_REVIEW_MODEL
from tools.logs_agent_ai.git_utils import filter_paths, get_changed_files, get_diff, parse_changed_lines
from tools.logs_agent_ai.llm import LLMClient, LLMError
from tools.logs_agent_ai.wiki import WikiPage, load_wiki_pages, select_review_pages


@dataclass
class ReviewFinding:
    path: str
    line: int
    severity: str
    title: str
    body: str
    wiki_refs: list[str]
    confidence: float


@dataclass
class PreparedComment:
    path: str
    line: int
    body: str


def build_review_prompts(selected_pages: list[WikiPage], diff_text: str, changed_paths: list[str]) -> tuple[str, str]:
    system_prompt = (
        "You are a specialized logs-agent architecture reviewer. "
        "Return only JSON with keys `summary` and `findings`. "
        "Each finding must be architecture-specific, grounded in the diff, and focused on correctness or delivery semantics."
    )
    page_blocks: list[str] = []
    for page in selected_pages:
        title = str(page.meta.get("title", page.path))
        summary = str(page.meta.get("summary", "")).strip()
        page_blocks.append(f"## {title} ({page.path})\nSummary: {summary}\n\n{page.body.strip()}")
    user_prompt = "\n".join(
        [
            "Review this logs-agent pull request diff against the wiki context below.",
            "",
            "Only report medium or higher risk findings that are specific to logs-agent architecture.",
            "Required checks:",
            "- input-to-single-pipeline ordering guarantees",
            "- sender reliable vs unreliable destination semantics",
            "- destination-to-auditor ack flow",
            "- auditor registry persistence, flush, restart, and duplicate/loss risks",
            "- tailer position and fingerprint recovery behavior",
            "- launcher/source/service interactions and config-driven path changes",
            "- graceful degradation and restart lifecycle behavior",
            "",
            f"Changed scoped paths: {', '.join(changed_paths)}",
            "",
            "JSON shape:",
            '{"summary":"...", "findings":[{"path":"...", "line":123, "severity":"medium|high|critical", "title":"...", "body":"...", "wiki_refs":["invariants/auditor-delivery.md"], "confidence":0.91}]}',
            "",
            "Wiki context:",
            "\n\n".join(page_blocks),
            "",
            "Diff:",
            "```diff",
            diff_text.rstrip(),
            "```",
        ]
    )
    return system_prompt, user_prompt


def validate_review_payload(payload: dict[str, object]) -> tuple[str, list[ReviewFinding]]:
    summary = payload.get("summary", "")
    findings_raw = payload.get("findings", [])
    if not isinstance(summary, str):
        raise LLMError("Review summary must be a string")
    if not isinstance(findings_raw, list):
        raise LLMError("Review findings must be a list")

    findings: list[ReviewFinding] = []
    for item in findings_raw:
        if not isinstance(item, dict):
            raise LLMError(f"Review finding must be an object: {item!r}")
        try:
            finding = ReviewFinding(
                path=str(item["path"]),
                line=int(item["line"]),
                severity=str(item["severity"]).lower(),
                title=str(item["title"]).strip(),
                body=str(item["body"]).strip(),
                wiki_refs=[str(ref) for ref in item.get("wiki_refs", [])],
                confidence=float(item["confidence"]),
            )
        except (KeyError, TypeError, ValueError) as error:
            raise LLMError(f"Malformed finding: {item!r}") from error
        findings.append(finding)
    return summary.strip(), findings


def prepare_review_comments(
    findings: list[ReviewFinding],
    changed_lines: dict[str, set[int]],
    scoped_paths: list[str],
    min_confidence: float = 0.74,
) -> tuple[list[PreparedComment], int]:
    comments: list[PreparedComment] = []
    suppressed = 0
    scoped = set(scoped_paths)
    for finding in findings:
        if finding.confidence < min_confidence or finding.path not in scoped:
            suppressed += 1
            continue
        available_lines = changed_lines.get(finding.path, set())
        comment_line = _nearest_changed_line(finding.line, available_lines)
        if comment_line is None:
            suppressed += 1
            continue
        comments.append(
            PreparedComment(
                path=finding.path,
                line=comment_line,
                body=format_inline_comment(finding),
            )
        )
    return comments, suppressed


def format_inline_comment(finding: ReviewFinding) -> str:
    refs = ", ".join(f"`{ref}`" for ref in finding.wiki_refs) if finding.wiki_refs else "wiki context"
    return f"**{finding.title}**\n\n{finding.body}\n\nContext: {refs}\nConfidence: {finding.confidence:.2f}"


def format_review_body(summary: str, comments: list[PreparedComment], suppressed: int) -> str:
    lines = [
        "Logs-agent architecture review",
        "",
        summary or "Reviewed the scoped logs-agent diff against the specialized wiki context.",
        "",
        f"Inline findings posted: {len(comments)}",
    ]
    if suppressed:
        lines.append(f"Suppressed low-confidence or unanchored findings: {suppressed}")
    return "\n".join(lines)


def submit_review(
    repository: str,
    pr_number: int,
    commit_sha: str,
    token: str,
    body: str,
    comments: list[PreparedComment],
) -> None:
    payload = {
        "body": body,
        "event": "COMMENT",
        "commit_id": commit_sha,
        "comments": [
            {
                "path": comment.path,
                "line": comment.line,
                "side": "RIGHT",
                "body": comment.body,
            }
            for comment in comments
        ],
    }
    request = urllib.request.Request(
        url=f"https://api.github.com/repos/{repository}/pulls/{pr_number}/reviews",
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request):
            return
    except urllib.error.HTTPError as error:
        message = error.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"GitHub review submission failed: {error.code} {message}") from error


def review_pull_request(
    repo_root: Path,
    repository: str,
    pr_number: int,
    base_sha: str,
    head_sha: str,
    commit_sha: str,
    scoped_globs: tuple[str, ...],
    llm_client: LLMClient,
) -> dict[str, object]:
    changed_paths = filter_paths(get_changed_files(repo_root, base_sha, head_sha), scoped_globs)
    if not changed_paths:
        return {"skipped": True, "reason": "no scoped files changed"}

    diff_text = get_diff(repo_root, base_sha, head_sha, changed_paths)
    changed_lines = parse_changed_lines(diff_text)
    pages = load_wiki_pages(repo_root)
    selected_pages = select_review_pages(changed_paths, pages, INVARIANT_PAGE_PATHS)
    system_prompt, user_prompt = build_review_prompts(selected_pages, diff_text, changed_paths)
    model = os.environ.get(REVIEW_MODEL_ENV) or DEFAULT_REVIEW_MODEL
    summary, findings = validate_review_payload(llm_client.generate_json(model, system_prompt, user_prompt))
    comments, suppressed = prepare_review_comments(findings, changed_lines, changed_paths)

    token = os.environ.get("GITHUB_TOKEN")
    if not token:
        raise RuntimeError("Missing GITHUB_TOKEN for review submission")

    body = format_review_body(summary, comments, suppressed)
    submit_review(repository, pr_number, commit_sha, token, body, comments)
    return {
        "skipped": False,
        "changed_paths": changed_paths,
        "selected_pages": [page.path for page in selected_pages],
        "findings_posted": len(comments),
        "suppressed_findings": suppressed,
    }


def _nearest_changed_line(requested_line: int, available_lines: set[int]) -> int | None:
    if not available_lines:
        return None
    if requested_line in available_lines:
        return requested_line
    nearest = min(available_lines, key=lambda line: abs(line - requested_line))
    if abs(nearest - requested_line) > 5:
        return None
    return nearest
