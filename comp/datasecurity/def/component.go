// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurity provides the data security agent component.
//
// The component subscribes to the DEBUG remote-config product. Each payload
// carries one or more scan tasks, each with a set of scanning rules and a
// per-integration scan query. The component:
//   - reconfigures the process-wide Sensitive Data Scanner (a single, unique
//     scanner) with the rules carried by the payload, so the rules are NOT
//     forwarded to the check config; the integration scans through the Agent's
//     scanner instead, and
//   - forwards the postgres scan query to the matching postgres config: it
//     takes over a postgres instance that opted in with data_security.enabled:
//     true, unschedules the original (file-provided) config and schedules an
//     enriched copy with the query merged into its data_security section.
//
// When the RC config goes away the original config is restored.
package datasecurity

// team: sensitive-data-scanner

// Component is the data security component interface.
//
// It currently exposes no methods because the component is purely
// side-effectful: it subscribes to RC on startup and reacts to received
// configs. Methods will be added as functionality grows.
type Component interface{}
