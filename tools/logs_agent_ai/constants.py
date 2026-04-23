from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

WIKI_ROOT = Path(".llm/logs-agent")
WIKI_DIRECTORIES = (
    "architecture",
    "components",
    "invariants",
    "configs",
    "playbooks",
    "adjacent",
    "queries",
)
SCHEMA_FILES = {"AGENTS.md", "index.md", "log.md"}
DEFAULT_WIKI_MODEL = "gpt-4.1-mini"
DEFAULT_REVIEW_MODEL = "gpt-4.1-mini"
DEFAULT_AI_BASE_URL = "https://api.openai.com/v1"
AI_API_KEY_ENV = "LOGS_AGENT_AI_API_KEY"
AI_BASE_URL_ENV = "LOGS_AGENT_AI_BASE_URL"
AI_SOURCE_ENV = "LOGS_AGENT_AI_SOURCE"
AI_ORG_ID_ENV = "LOGS_AGENT_AI_ORG_ID"
WIKI_MODEL_ENV = "LOGS_AGENT_WIKI_MODEL"
REVIEW_MODEL_ENV = "LOGS_AGENT_REVIEW_MODEL"

SCOPED_REVIEW_GLOBS = (
    "pkg/logs/**",
    "comp/logs/**",
    "comp/logs-library/**",
    "cmd/agent/subcommands/streamlogs/**",
    "cmd/agent/subcommands/analyzelogs/**",
)


@dataclass(frozen=True)
class PageSpec:
    path: str
    title: str
    kind: str
    summary: str
    source_paths: tuple[str, ...]
    owns_globs: tuple[str, ...]
    related_pages: tuple[str, ...]
    prompt_hint: str


PAGE_SPECS = (
    PageSpec(
        path="architecture/logs-agent-overview.md",
        title="Logs Agent Overview",
        kind="architecture",
        summary="High-level map of how sources, pipelines, destinations, and the auditor fit together.",
        source_paths=("pkg/logs/README.md", "comp/logs/agent/agentimpl/agent.go"),
        owns_globs=("pkg/logs/README.md", "comp/logs/agent/agentimpl/agent.go"),
        related_pages=(
            "architecture/pipeline-flow.md",
            "architecture/source-discovery.md",
            "invariants/pipeline-ordering.md",
        ),
        prompt_hint="Capture the overall logs-agent topology and the strongest architectural boundaries.",
    ),
    PageSpec(
        path="architecture/pipeline-flow.md",
        title="Pipeline Flow",
        kind="architecture",
        summary="Tailer-to-auditor data flow, including processor, strategy, sender, destination, and retry boundaries.",
        source_paths=(
            "pkg/logs/README.md",
            "pkg/logs/sender/sender.go",
            "pkg/logs/sender/worker.go",
            "pkg/logs/client/http/destination.go",
        ),
        owns_globs=("pkg/logs/sender/**", "pkg/logs/client/**", "pkg/logs/processor/**"),
        related_pages=(
            "components/sender.md",
            "components/processor.md",
            "components/auditor.md",
            "invariants/sender-destination-semantics.md",
            "invariants/auditor-delivery.md",
        ),
        prompt_hint="Explain the hot path for payload delivery and where reliability or ordering assumptions live.",
    ),
    PageSpec(
        path="architecture/source-discovery.md",
        title="Source Discovery and Launchers",
        kind="architecture",
        summary="How schedulers, sources, services, and launchers cooperate to create tailers and attach them to pipelines.",
        source_paths=(
            "pkg/logs/README.md",
            "pkg/logs/launchers/README.md",
            "pkg/logs/schedulers/README.md",
            "comp/logs/agent/agentimpl/agent_core_init.go",
        ),
        owns_globs=("pkg/logs/launchers/**", "pkg/logs/schedulers/**", "pkg/logs/sources/**"),
        related_pages=(
            "components/launchers.md",
            "invariants/launcher-source-service-contracts.md",
            "configs/logs-agent-config-flags.md",
        ),
        prompt_hint="Focus on discovery fan-in, launcher responsibilities, and configuration-driven tailer selection.",
    ),
    PageSpec(
        path="components/auditor.md",
        title="Auditor Component",
        kind="component",
        summary="Registry-backed persistence for offsets, tailing state, fingerprints, and restart safety.",
        source_paths=(
            "comp/logs/auditor/def/component.go",
            "comp/logs/auditor/def/types.go",
            "comp/logs/auditor/impl/auditor.go",
            "comp/logs/auditor/impl/registry_writer.go",
        ),
        owns_globs=("comp/logs/auditor/**",),
        related_pages=(
            "invariants/auditor-delivery.md",
            "invariants/tailer-position-and-fingerprint.md",
            "components/restart-lifecycle.md",
        ),
        prompt_hint="Track how offsets become durable state and which failure modes can create duplicate logs or data loss.",
    ),
    PageSpec(
        path="components/sender.md",
        title="Sender and Destinations",
        kind="component",
        summary="Reliable vs unreliable destination behavior, sender workers, buffering, and retry interaction.",
        source_paths=(
            "pkg/logs/sender/sender.go",
            "pkg/logs/sender/worker.go",
            "pkg/logs/client/destinations.go",
            "pkg/logs/client/http/destination.go",
        ),
        owns_globs=("pkg/logs/sender/**", "pkg/logs/client/**"),
        related_pages=(
            "invariants/sender-destination-semantics.md",
            "invariants/auditor-delivery.md",
            "architecture/pipeline-flow.md",
        ),
        prompt_hint="Emphasize delivery guarantees, backpressure, MRF handling, and how auditor updates are preserved.",
    ),
    PageSpec(
        path="components/processor.md",
        title="Processor and Message Mutation",
        kind="component",
        summary="Where message filtering, enrichment, encoding, and processing rules are applied before sending.",
        source_paths=(
            "pkg/logs/processor/processor.go",
            "pkg/logs/processor/encoder.go",
            "pkg/logs/message/message.go",
            "pkg/logs/message/origin.go",
        ),
        owns_globs=("pkg/logs/processor/**", "pkg/logs/message/**"),
        related_pages=(
            "architecture/pipeline-flow.md",
            "configs/logs-agent-config-flags.md",
        ),
        prompt_hint="Describe which mutations are safe in the processor layer and which fields downstream components depend on.",
    ),
    PageSpec(
        path="components/launchers.md",
        title="Launchers and Tailers",
        kind="component",
        summary="Launcher-specific behavior for file, container, journald, listeners, and Windows event ingestion.",
        source_paths=(
            "pkg/logs/launchers/README.md",
            "pkg/logs/tailers/README.md",
            "pkg/logs/launchers/file/position.go",
            "comp/logs/agent/agentimpl/agent_core_init.go",
        ),
        owns_globs=("pkg/logs/launchers/**", "pkg/logs/tailers/**"),
        related_pages=(
            "architecture/source-discovery.md",
            "invariants/launcher-source-service-contracts.md",
            "invariants/tailer-position-and-fingerprint.md",
        ),
        prompt_hint="Note launcher-specific behaviors that commonly regress when source or config plumbing changes.",
    ),
    PageSpec(
        path="components/restart-lifecycle.md",
        title="Restart Lifecycle",
        kind="component",
        summary="Partial restart flow, transport switching, persistent state reuse, and graceful stop expectations.",
        source_paths=(
            "comp/logs/agent/agentimpl/agent_restart.go",
            "comp/logs/agent/agentimpl/agent_core_init.go",
            "comp/logs/agent/agentimpl/agent.go",
        ),
        owns_globs=("comp/logs/agent/agentimpl/**",),
        related_pages=(
            "invariants/graceful-restart.md",
            "components/auditor.md",
            "components/sender.md",
        ),
        prompt_hint="Document which components persist across restart and which are rebuilt, especially around transport upgrades.",
    ),
    PageSpec(
        path="invariants/pipeline-ordering.md",
        title="Pipeline Ordering Invariants",
        kind="invariant",
        summary="Each input stays pinned to one pipeline so message order is preserved for that input.",
        source_paths=("pkg/logs/README.md", "comp/logs-library/pipeline", "pkg/logs/sender/sender.go"),
        owns_globs=("pkg/logs/**", "comp/logs-library/pipeline/**"),
        related_pages=(
            "architecture/logs-agent-overview.md",
            "architecture/pipeline-flow.md",
            "playbooks/review-checklist.md",
        ),
        prompt_hint="Capture the ordering assumptions that should block architectural shortcuts during reviews.",
    ),
    PageSpec(
        path="invariants/sender-destination-semantics.md",
        title="Sender and Destination Semantics",
        kind="invariant",
        summary="Reliable destinations gate progress; unreliable ones do not block or update the auditor.",
        source_paths=("pkg/logs/sender/worker.go", "pkg/logs/client/destinations.go", "pkg/logs/client/http/destination.go"),
        owns_globs=("pkg/logs/sender/**", "pkg/logs/client/**"),
        related_pages=(
            "components/sender.md",
            "invariants/auditor-delivery.md",
            "playbooks/review-checklist.md",
        ),
        prompt_hint="Make the reliable versus unreliable destination contract explicit for reviewers.",
    ),
    PageSpec(
        path="invariants/auditor-delivery.md",
        title="Auditor Delivery and Persistence",
        kind="invariant",
        summary="Only successfully delivered reliable payloads should advance auditor state, and restart paths must flush that state safely.",
        source_paths=(
            "pkg/logs/README.md",
            "pkg/logs/sender/worker.go",
            "comp/logs/auditor/impl/auditor.go",
            "comp/logs/agent/agentimpl/agent_restart.go",
        ),
        owns_globs=("pkg/logs/sender/**", "comp/logs/auditor/**", "comp/logs/agent/agentimpl/**"),
        related_pages=(
            "components/auditor.md",
            "components/restart-lifecycle.md",
            "playbooks/review-checklist.md",
        ),
        prompt_hint="Center duplicate-log and lost-ack failure modes involving the auditor, sender, and restart sequence.",
    ),
    PageSpec(
        path="invariants/tailer-position-and-fingerprint.md",
        title="Tailer Position and Fingerprint Recovery",
        kind="invariant",
        summary="Tailers recover positions from the auditor registry, with fingerprint checks protecting against log rotation mistakes.",
        source_paths=("pkg/logs/launchers/file/position.go", "comp/logs/auditor/impl/auditor.go"),
        owns_globs=("pkg/logs/launchers/file/**", "comp/logs/auditor/**"),
        related_pages=(
            "components/launchers.md",
            "components/auditor.md",
            "playbooks/review-checklist.md",
        ),
        prompt_hint="Call out any change that could break restart position recovery, rotation handling, or fingerprint safety.",
    ),
    PageSpec(
        path="invariants/launcher-source-service-contracts.md",
        title="Launcher Source and Service Contracts",
        kind="invariant",
        summary="Launchers rely on source and service stores differently, especially for container collection and autodiscovery timing.",
        source_paths=("pkg/logs/README.md", "pkg/logs/schedulers/README.md", "comp/logs/agent/agentimpl/agent_core_init.go"),
        owns_globs=("pkg/logs/launchers/**", "pkg/logs/schedulers/**", "pkg/logs/service/**", "pkg/logs/sources/**"),
        related_pages=(
            "architecture/source-discovery.md",
            "components/launchers.md",
            "configs/logs-agent-config-flags.md",
        ),
        prompt_hint="Make service/source race conditions and launcher-specific contracts visible to reviewers.",
    ),
    PageSpec(
        path="invariants/graceful-restart.md",
        title="Graceful Restart Invariants",
        kind="invariant",
        summary="Partial restart must preserve persistent components, flush offsets, and avoid duplicate retry loops or dropped state.",
        source_paths=("comp/logs/agent/agentimpl/agent_restart.go", "comp/logs/agent/agentimpl/agent.go"),
        owns_globs=("comp/logs/agent/agentimpl/**",),
        related_pages=(
            "components/restart-lifecycle.md",
            "components/auditor.md",
            "playbooks/review-checklist.md",
        ),
        prompt_hint="Describe the non-obvious lifecycle guarantees around partial stop, rebuild, rollback, and retry loops.",
    ),
    PageSpec(
        path="configs/logs-agent-config-flags.md",
        title="Logs Agent Config Flags with Architectural Side Effects",
        kind="config",
        summary="A curated list of config flags that change delivery semantics, tailer selection, restart behavior, or registry persistence.",
        source_paths=(
            "pkg/logs/README.md",
            "comp/logs/auditor/impl/auditor.go",
            "comp/logs/agent/agentimpl/agent_core_init.go",
            "pkg/logs/launchers/container/tailerfactory/whichtailer.go",
        ),
        owns_globs=("comp/logs/agent/config/**", "comp/logs/auditor/**", "pkg/logs/launchers/**"),
        related_pages=(
            "architecture/source-discovery.md",
            "invariants/launcher-source-service-contracts.md",
        ),
        prompt_hint="Focus on flags that quietly change contracts rather than exhaustive configuration coverage.",
    ),
    PageSpec(
        path="playbooks/review-checklist.md",
        title="Logs Agent Review Checklist",
        kind="playbook",
        summary="Reviewer checklist for architectural regressions in delivery guarantees, offsets, restarts, and launcher behavior.",
        source_paths=("pkg/logs/README.md", "comp/logs/agent/agentimpl/agent_restart.go", "comp/logs/auditor/impl/auditor.go"),
        owns_globs=("pkg/logs/**", "comp/logs/**"),
        related_pages=(
            "invariants/pipeline-ordering.md",
            "invariants/sender-destination-semantics.md",
            "invariants/auditor-delivery.md",
            "invariants/tailer-position-and-fingerprint.md",
            "invariants/graceful-restart.md",
        ),
        prompt_hint="Produce a concise architecture-first review rubric, not a generic code review checklist.",
    ),
    PageSpec(
        path="adjacent/streamlogs-and-analyzelogs.md",
        title="Streamlogs and Analyzelogs Adjacent Interfaces",
        kind="adjacent",
        summary="CLI entrypoints and adjacent logs components that expose or consume logs-agent behavior.",
        source_paths=(
            "cmd/agent/subcommands/streamlogs/command.go",
            "cmd/agent/subcommands/analyzelogs/command.go",
            "comp/logs/streamlogs/impl/streamlogs.go",
        ),
        owns_globs=("cmd/agent/subcommands/streamlogs/**", "cmd/agent/subcommands/analyzelogs/**", "comp/logs/streamlogs/**"),
        related_pages=(
            "architecture/logs-agent-overview.md",
            "components/restart-lifecycle.md",
        ),
        prompt_hint="Describe nearby operator-facing interfaces that may break when internal contracts shift.",
    ),
    PageSpec(
        path="queries/high-risk-review-questions.md",
        title="High-Risk Review Questions",
        kind="query",
        summary="Saved set of questions the bot should ask itself when reviewing logs-agent pull requests.",
        source_paths=("pkg/logs/README.md", "comp/logs/auditor/impl/auditor.go", "pkg/logs/sender/worker.go"),
        owns_globs=("pkg/logs/**", "comp/logs/**"),
        related_pages=(
            "playbooks/review-checklist.md",
            "invariants/auditor-delivery.md",
            "invariants/graceful-restart.md",
        ),
        prompt_hint="Write durable review prompts the bot can reuse when triaging architecture-sensitive diffs.",
    ),
)

PAGE_SPECS_BY_PATH = {spec.path: spec for spec in PAGE_SPECS}
INVARIANT_PAGE_PATHS = tuple(spec.path for spec in PAGE_SPECS if spec.kind == "invariant")
