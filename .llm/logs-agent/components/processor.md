---
title: "Processor and Message Mutation"
kind: "component"
summary: "Where message filtering, enrichment, encoding, and processing rules are applied before sending."
source_paths:
  - "pkg/logs/processor/processor.go"
  - "pkg/logs/processor/encoder.go"
  - "pkg/logs/message/message.go"
  - "pkg/logs/message/origin.go"
owns_globs:
  - "pkg/logs/processor/**"
  - "pkg/logs/message/**"
related_pages:
  - "architecture/pipeline-flow.md"
  - "configs/logs-agent-config-flags.md"
last_ingested_sha: "HEAD"
---

The processor layer is a pivotal component in the logs agent architecture, responsible for normalizing, enriching, and encoding messages before they are dispatched downstream. This layer applies various processing rules that can significantly alter the semantics of messages for all components that follow.

## Key Responsibilities

- **Message Normalization**: Ensures messages conform to expected formats.
- **Enrichment**: Adds additional metadata to messages, enhancing their context.
- **Encoding**: Prepares messages for transport by converting them into a suitable format.

## Safe Mutations

Certain mutations are considered safe within the processor layer:

- **Filtering**: Removing unnecessary fields or content that does not affect downstream processing.
- **Enrichment**: Adding metadata that does not alter the original message content.
- **Encoding**: Transforming messages into a transport-ready format while preserving original content.

## Critical Fields for Downstream Components

Downstream components depend on specific fields for proper functioning:

- **Message Origin Fields**: Essential for maintaining identity and routing (e.g., `message.Origin.Identifier`).
- **Message Content**: The core data that must remain intact for accurate processing and diagnostics.
- **Processing Tags**: Used for categorization and filtering in downstream analytics.

## Review Traps

When reviewing changes in the processor layer, be cautious of the following:

- **Field Changes**: Modifications to `message.Message` or `message.Origin` can disrupt sender routing, auditor identifiers, or diagnostics.
- **Assumptions of Stability**: Downstream components may treat certain fields as stable contracts; changes can lead to unexpected behavior.

For a comprehensive understanding of the message flow through the logs agent, see [Pipeline Flow](../architecture/pipeline-flow.md). For insights into configuration impacts on architecture, refer to [Logs Agent Config Flags with Architectural Side Effects](../configs/logs-agent-config-flags.md).
