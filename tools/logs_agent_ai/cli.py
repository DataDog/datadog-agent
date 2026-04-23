from __future__ import annotations

import argparse
import os
from pathlib import Path

from tools.logs_agent_ai.constants import (
    DEFAULT_WIKI_MODEL,
    SCOPED_REVIEW_GLOBS,
    WIKI_MODEL_ENV,
)
from tools.logs_agent_ai.git_utils import get_changed_files, short_sha
from tools.logs_agent_ai.llm import LLMClient
from tools.logs_agent_ai.review import review_pull_request
from tools.logs_agent_ai.wiki import (
    append_log,
    build_ingest_prompts,
    determine_impacted_page_specs,
    initialize_schema_files,
    refresh_index,
    write_page,
)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Logs-agent wiki and review automation")
    subparsers = parser.add_subparsers(dest="command", required=True)

    ingest = subparsers.add_parser("ingest", help="Update the logs-agent wiki for changed source files")
    ingest.add_argument("--base-sha", required=True)
    ingest.add_argument("--head-sha", required=True)

    review = subparsers.add_parser("review-pr", help="Review a PR with logs-agent wiki context")
    review.add_argument("--pr", type=int, required=True)
    review.add_argument("--base-sha", required=True)
    review.add_argument("--head-sha", required=True)
    review.add_argument("--commit", required=True)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    repo_root = Path.cwd()
    if args.command == "ingest":
        return run_ingest(repo_root, args.base_sha, args.head_sha)
    if args.command == "review-pr":
        repository = os.environ.get("GITHUB_REPOSITORY")
        if not repository:
            raise RuntimeError("Missing GITHUB_REPOSITORY")
        return run_review(repo_root, repository, args.pr, args.base_sha, args.head_sha, args.commit)
    raise RuntimeError(f"Unknown command: {args.command}")


def run_ingest(repo_root: Path, base_sha: str, head_sha: str) -> int:
    initialize_schema_files(repo_root)
    changed_paths = get_changed_files(repo_root, base_sha, head_sha)
    impacted_specs = determine_impacted_page_specs(changed_paths)
    llm_client = LLMClient()
    model = os.environ.get(WIKI_MODEL_ENV) or DEFAULT_WIKI_MODEL

    changed_pages: list[str] = []
    for spec in impacted_specs:
        system_prompt, user_prompt = build_ingest_prompts(repo_root, spec, changed_paths, head_sha)
        body = llm_client.generate_markdown(model, system_prompt, user_prompt)
        if body.startswith("```"):
            lines = body.splitlines()
            body = "\n".join(lines[1:-1]).strip()
        write_page(repo_root, spec, body, head_sha)
        changed_pages.append(spec.path)

    refresh_index(repo_root)
    append_log(
        repo_root,
        "ingest",
        f"refresh for {short_sha(head_sha)}",
        head_sha,
        [
            f"base_sha: `{base_sha}`",
            f"changed_source_paths: {', '.join(changed_paths) if changed_paths else 'none'}",
            f"updated_pages: {', '.join(changed_pages) if changed_pages else 'none'}",
        ],
    )
    print(f"Updated {len(changed_pages)} wiki pages")
    return 0


def run_review(repo_root: Path, repository: str, pr_number: int, base_sha: str, head_sha: str, commit_sha: str) -> int:
    llm_client = LLMClient()
    result = review_pull_request(
        repo_root=repo_root,
        repository=repository,
        pr_number=pr_number,
        base_sha=base_sha,
        head_sha=head_sha,
        commit_sha=commit_sha,
        scoped_globs=SCOPED_REVIEW_GLOBS,
        llm_client=llm_client,
    )
    if result.get("skipped"):
        print(f"Skipped review: {result['reason']}")
        return 0
    print(
        "Posted logs-agent review with "
        f"{result['findings_posted']} findings and {result['suppressed_findings']} suppressed findings"
    )
    return 0
