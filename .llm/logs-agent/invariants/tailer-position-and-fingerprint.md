---
title: "Tailer Position and Fingerprint Recovery"
kind: "invariant"
summary: "Tailers recover positions from the auditor registry, with fingerprint checks protecting against log rotation mistakes."
source_paths:
  - "pkg/logs/launchers/file/position.go"
  - "comp/logs/auditor/impl/auditor.go"
owns_globs:
  - "pkg/logs/launchers/file/**"
  - "comp/logs/auditor/**"
related_pages:
  - "components/launchers.md"
  - "components/auditor.md"
  - "playbooks/review-checklist.md"
last_ingested_sha: "HEAD"
---

# Tailer Position and Fingerprint Recovery

Tailers recover positions from the auditor registry, utilizing fingerprint checks to prevent incorrect resumption of rotated logs. This mechanism is crucial for maintaining data integrity during log ingestion.

## Hard Rules

- **Stored Offsets**: Only reused when fingerprints align.
- **Forced Tailing Modes**: Can intentionally override stored positions.
- **Mismatched Fingerprints**: Should prioritize safe recovery over blind reuse.

## Review Traps

- **Identifier Changes**: Alterations to identifiers or fingerprint computation can lead to duplicate collections or data loss after rotation or restart.
- **Tailing Mode Precedence**: Changes in tailing mode handling may affect recovery behavior.

## Invariants

- **Position Recovery**: Tailers must accurately recover positions based on the auditor registry.
- **Fingerprint Safety**: Fingerprints must be checked to ensure logs are not resumed from incorrect positions after rotation.

## Related Components

- See [Auditor Component](../components/auditor.md) for details on how the auditor manages log offsets and fingerprints.
- Refer to [Launchers and Tailers](../components/launchers.md) for insights on how tailers interact with the log ingestion pipeline.

## Summary

The recovery process relies on matching stored offsets to the correct file identity. Fingerprints serve as a safeguard against resuming from incorrect positions after log rotation. Ensuring that the fingerprinting mechanism is robust is essential for reliable log processing.

For any changes that could impact restart position recovery, rotation handling, or fingerprint safety, be vigilant in your reviews to maintain the integrity of the logs-agent architecture.
