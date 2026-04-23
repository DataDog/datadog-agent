from __future__ import annotations

import datetime as dt
import fnmatch
import os
from dataclasses import dataclass
from pathlib import Path

from tools.logs_agent_ai.constants import (
    PAGE_SPECS,
    PAGE_SPECS_BY_PATH,
    SCHEMA_FILES,
    WIKI_DIRECTORIES,
    WIKI_ROOT,
    PageSpec,
)
from tools.logs_agent_ai.frontmatter import dump_frontmatter, parse_frontmatter


@dataclass
class WikiPage:
    path: str
    meta: dict[str, object]
    body: str


def ensure_wiki_layout(repo_root: Path) -> None:
    root = repo_root / WIKI_ROOT
    root.mkdir(parents=True, exist_ok=True)
    for directory in WIKI_DIRECTORIES:
        (root / directory).mkdir(parents=True, exist_ok=True)


def load_wiki_pages(repo_root: Path) -> dict[str, WikiPage]:
    root = repo_root / WIKI_ROOT
    pages: dict[str, WikiPage] = {}
    if not root.exists():
        return pages
    for path in sorted(root.rglob("*.md")):
        rel = path.relative_to(root).as_posix()
        if rel in SCHEMA_FILES:
            continue
        meta, body = parse_frontmatter(path.read_text())
        pages[rel] = WikiPage(path=rel, meta=meta, body=body.strip())
    return pages


def build_page_document(spec: PageSpec, body: str, head_sha: str) -> str:
    meta = {
        "title": spec.title,
        "kind": spec.kind,
        "summary": spec.summary,
        "source_paths": list(spec.source_paths),
        "owns_globs": list(spec.owns_globs),
        "related_pages": list(spec.related_pages),
        "last_ingested_sha": head_sha,
    }
    content = dump_frontmatter(meta)
    return f"{content}\n\n{body.rstrip()}\n"


def write_page(repo_root: Path, spec: PageSpec, body: str, head_sha: str) -> Path:
    path = repo_root / WIKI_ROOT / spec.path
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(build_page_document(spec, body, head_sha))
    return path


def determine_impacted_page_specs(changed_paths: list[str]) -> list[PageSpec]:
    if not changed_paths:
        return list(PAGE_SPECS)
    impacted: list[PageSpec] = []
    for spec in PAGE_SPECS:
        if any(fnmatch.fnmatch(path, pattern) for path in changed_paths for pattern in spec.owns_globs):
            impacted.append(spec)
    return impacted


def select_review_pages(changed_paths: list[str], pages: dict[str, WikiPage], required_pages: tuple[str, ...]) -> list[WikiPage]:
    selected: list[WikiPage] = []
    seen: set[str] = set()
    for rel_path, page in pages.items():
        owns_globs = tuple(page.meta.get("owns_globs", []))  # type: ignore[arg-type]
        if any(fnmatch.fnmatch(path, pattern) for path in changed_paths for pattern in owns_globs):
            selected.append(page)
            seen.add(rel_path)
    for rel_path in required_pages:
        page = pages.get(rel_path)
        if page and rel_path not in seen:
            selected.append(page)
            seen.add(rel_path)
    return selected


def refresh_index(repo_root: Path) -> None:
    pages = load_wiki_pages(repo_root)
    grouped: dict[str, list[WikiPage]] = {}
    for page in pages.values():
        grouped.setdefault(str(page.meta.get("kind", "misc")), []).append(page)
    lines = [
        "# Logs Agent Wiki Index",
        "",
        "This index is rebuilt during wiki ingests. Start here to find architecture, invariants, components, and reusable review questions.",
        "",
    ]
    for kind in sorted(grouped):
        heading = kind.capitalize()
        lines.append(f"## {heading}")
        lines.append("")
        for page in sorted(grouped[kind], key=lambda item: item.path):
            title = str(page.meta.get("title", page.path))
            summary = str(page.meta.get("summary", "")).strip()
            lines.append(f"- [{title}]({page.path}) - {summary}")
        lines.append("")
    (repo_root / WIKI_ROOT / "index.md").write_text("\n".join(lines).rstrip() + "\n")


def append_log(repo_root: Path, entry_type: str, title: str, head_sha: str, details: list[str]) -> None:
    path = repo_root / WIKI_ROOT / "log.md"
    timestamp = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%d")
    entry = [f"## [{timestamp}] {entry_type} | {title}", "", f"- head_sha: `{head_sha}`"]
    entry.extend(f"- {detail}" for detail in details)
    entry.append("")
    existing = path.read_text() if path.exists() else "# Logs Agent Wiki Log\n\n"
    path.write_text(existing.rstrip() + "\n\n" + "\n".join(entry))


def gather_source_snippets(repo_root: Path, spec: PageSpec, changed_paths: list[str], max_files: int = 8, max_chars: int = 20000) -> list[tuple[str, str]]:
    candidates: list[str] = []
    for source in spec.source_paths:
        absolute = repo_root / source
        if absolute.is_file():
            candidates.append(source)
            continue
        if absolute.is_dir():
            for child in sorted(absolute.rglob("*")):
                if not child.is_file():
                    continue
                rel = child.relative_to(repo_root).as_posix()
                if _is_source_file(rel):
                    candidates.append(rel)
    prioritized = list(dict.fromkeys(
        [path for path in changed_paths if any(fnmatch.fnmatch(path, pattern) for pattern in spec.owns_globs)] + candidates
    ))
    snippets: list[tuple[str, str]] = []
    total_chars = 0
    for rel_path in prioritized:
        absolute = repo_root / rel_path
        if not absolute.is_file():
            continue
        try:
            content = absolute.read_text()
        except UnicodeDecodeError:
            continue
        excerpt = content[: min(len(content), 4000)]
        if total_chars + len(excerpt) > max_chars and snippets:
            break
        snippets.append((rel_path, excerpt))
        total_chars += len(excerpt)
        if len(snippets) >= max_files:
            break
    return snippets


def build_ingest_prompts(repo_root: Path, spec: PageSpec, changed_paths: list[str], head_sha: str) -> tuple[str, str]:
    schema = (repo_root / WIKI_ROOT / "AGENTS.md").read_text()
    current_path = repo_root / WIKI_ROOT / spec.path
    current_body = ""
    if current_path.exists():
        _, current_body = parse_frontmatter(current_path.read_text())
    source_snippets = gather_source_snippets(repo_root, spec, changed_paths)
    system_prompt = (
        "You maintain a logs-agent architecture wiki. "
        "Return markdown body only. Do not include YAML frontmatter. "
        "Prefer concise sections, bullet lists for invariants, and markdown links to related wiki pages."
    )
    lines = [
        f"Schema:\n\n{schema}",
        "",
        f"Target page: {spec.path}",
        f"Title: {spec.title}",
        f"Kind: {spec.kind}",
        f"Summary: {spec.summary}",
        f"Prompt focus: {spec.prompt_hint}",
        f"Head SHA: {head_sha}",
        f"Changed source paths: {', '.join(changed_paths) if changed_paths else 'seed / full ingest'}",
        "",
        "Existing page body:",
        current_body.strip() or "(none)",
        "",
        "Source snippets:",
    ]
    for rel_path, snippet in source_snippets:
        lines.append(f"### {rel_path}")
        lines.append("```text")
        lines.append(snippet.rstrip())
        lines.append("```")
        lines.append("")
    lines.append("Rewrite the page so it is durable, architecture-focused, and cross-referenced.")
    return system_prompt, "\n".join(lines).strip() + "\n"


def initialize_schema_files(repo_root: Path) -> None:
    ensure_wiki_layout(repo_root)
    log_path = repo_root / WIKI_ROOT / "log.md"
    if not log_path.exists():
        log_path.write_text("# Logs Agent Wiki Log\n")
    index_path = repo_root / WIKI_ROOT / "index.md"
    if not index_path.exists():
        index_path.write_text("# Logs Agent Wiki Index\n")


def _is_source_file(path: str) -> bool:
    _, extension = os.path.splitext(path)
    return extension in {".go", ".md", ".py", ".yaml", ".yml"}
