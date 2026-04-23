# Logs Agent Wiki Index

## Architecture

- [Logs Agent Overview](architecture/logs-agent-overview.md) - High-level map of how sources, pipelines, destinations, and the auditor fit together.
- [Pipeline Flow](architecture/pipeline-flow.md) - Tailer-to-auditor data flow, including processor, strategy, sender, destination, and retry boundaries.
- [Source Discovery and Launchers](architecture/source-discovery.md) - How schedulers, sources, services, and launchers cooperate to create tailers and attach them to pipelines.

## Components

- [Auditor Component](components/auditor.md) - Registry-backed persistence for offsets, tailing state, fingerprints, and restart safety.
- [Launchers and Tailers](components/launchers.md) - Launcher-specific behavior for file, container, journald, listeners, and Windows event ingestion.
- [Processor and Message Mutation](components/processor.md) - Where message filtering, enrichment, encoding, and processing rules are applied before sending.
- [Restart Lifecycle](components/restart-lifecycle.md) - Partial restart flow, transport switching, persistent state reuse, and graceful stop expectations.
- [Sender and Destinations](components/sender.md) - Reliable vs unreliable destination behavior, sender workers, buffering, and retry interaction.

## Invariants

- [Auditor Delivery and Persistence](invariants/auditor-delivery.md) - Only successful reliable deliveries should advance auditor state.
- [Graceful Restart Invariants](invariants/graceful-restart.md) - Partial restart must preserve persistent state and avoid duplicate retry behavior.
- [Launcher Source and Service Contracts](invariants/launcher-source-service-contracts.md) - Launchers depend on source and service stores in different ways.
- [Pipeline Ordering Invariants](invariants/pipeline-ordering.md) - Each input stays pinned to one pipeline so order is preserved.
- [Sender and Destination Semantics](invariants/sender-destination-semantics.md) - Reliable destinations gate progress; unreliable ones do not update the auditor.
- [Tailer Position and Fingerprint Recovery](invariants/tailer-position-and-fingerprint.md) - Tailers recover offsets from the auditor registry with fingerprint safeguards.

## Configs

- [Logs Agent Config Flags with Architectural Side Effects](configs/logs-agent-config-flags.md) - Config switches that alter delivery semantics, tailer selection, or persistence.

## Playbooks

- [Logs Agent Review Checklist](playbooks/review-checklist.md) - Reusable checklist for architecture-sensitive PR review.

## Adjacent

- [Streamlogs and Analyzelogs Adjacent Interfaces](adjacent/streamlogs-and-analyzelogs.md) - Nearby operator-facing interfaces that expose logs-agent behavior.

## Queries

- [High-Risk Review Questions](queries/high-risk-review-questions.md) - Reusable questions the review bot should ask itself on risky PRs.
