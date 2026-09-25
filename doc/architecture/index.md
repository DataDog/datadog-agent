---
title: Architecture
---

# Architecture

Explore how the Datadog Agent is organized, how its configuration is defined, and how its components process telemetry.

## Component framework

The [component framework](components/index.md) separates features behind interfaces and assembles them into Agent binaries. Read about [Fx](components/fx.md) for dependency injection and application lifecycle management.

## Agent schema

The [Agent configuration schema](agent-schema/index.md) defines configuration settings, validation, and generated documentation. The [annotated examples](agent-schema/examples.md) explain how settings and sections fit together.

## DogStatsD

Read about [DogStatsD internals](dogstatsd/internals.md) to follow metrics from incoming packets through processing and aggregation.
