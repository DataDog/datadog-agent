# Logs Agent Wiki Schema

This directory is a persistent LLM-maintained wiki for the Datadog logs agent. It sits between raw source code and review/query workflows.

## Purpose

- Keep architecture knowledge cumulative instead of rediscovered from raw code on every review.
- Focus only on the logs-agent domain and adjacent entrypoints that materially affect it.
- Preserve the codebase as the source of truth. Wiki pages summarize and synthesize; they do not replace source code.

## Structure

- `index.md`: content-oriented catalog of wiki pages.
- `log.md`: append-only timeline of ingests and notable review/query updates.
- `architecture/`: end-to-end flow pages and topological maps.
- `components/`: component contracts, lifecycle notes, and responsibilities.
- `invariants/`: non-negotiable delivery, ordering, restart, and persistence guarantees.
- `configs/`: config switches with architectural side effects.
- `playbooks/`: reusable review and debugging checklists.
- `adjacent/`: nearby interfaces that expose or depend on logs-agent behavior.
- `queries/`: reusable query prompts or saved analyses worth preserving.

## Page Contract

Every wiki page under subdirectories must include YAML frontmatter with:

- `title`
- `kind`
- `summary`
- `source_paths`
- `owns_globs`
- `related_pages`
- `last_ingested_sha`

The page body should:

- start with a short overview
- use bullets for invariants, failure modes, and review traps
- link to related wiki pages with relative markdown links
- cite repo paths inline when referencing code
- avoid large verbatim source excerpts

## Ingest Workflow

When processing new source changes:

1. Read the changed raw sources first.
2. Map changed paths to impacted wiki pages using `owns_globs`.
3. Rewrite impacted pages in place rather than creating near-duplicate pages.
4. Preserve stable filenames.
5. Update `last_ingested_sha` to the new head commit SHA.
6. Refresh `index.md`.
7. Append a chronological entry to `log.md`.

Ingest should prefer synthesis over exhaustiveness. Capture:

- architecture boundaries
- data flow and ownership
- invariants
- subtle config side effects
- common failure modes
- reviewer heuristics

## Query Workflow

When answering questions:

1. Read `index.md`.
2. Load the smallest set of relevant pages.
3. Always include the invariant pages if the question is about PR risk or architectural changes.
4. Synthesize from wiki pages first, then verify against raw code if needed.
5. If the answer creates durable knowledge, file it back into `queries/` or update an existing page.

## Review Workflow

PR reviews must be architecture-first, not style-first. Always test the diff against:

- input-to-single-pipeline ordering guarantees
- sender reliable vs unreliable destination semantics
- destination-to-auditor ack flow
- auditor registry persistence, flush, restart, and duplicate/loss risks
- tailer position and fingerprint recovery behavior
- launcher/source/service interactions and config-driven path changes
- graceful degradation and restart lifecycle behavior

Inline comments should only be posted when the finding is:

- specific to the changed code
- grounded in logs-agent architecture
- at least medium severity
- high-confidence enough to avoid review noise

## Maintenance Rules

- Prefer editing existing pages over adding new ones unless the concept is clearly distinct.
- Keep summaries durable across refactors.
- Flag contradictions between pages during ingest when source changes invalidate older assumptions.
- Do not copy large source blocks into the wiki.
