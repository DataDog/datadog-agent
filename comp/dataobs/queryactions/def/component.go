// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package queryactions provides the Data Observability query actions component
package queryactions

// team: data-observability

// Component is the Data Observability query actions component interface.
// This component subscribes to RC DO_QUERY_ACTIONS product to receive declarative query configs,
// each containing the full set of active monitor queries for a DB instance.
// It schedules long-lived checks that manage query scheduling internally.
// Gated by the data_observability.query_actions.enabled configuration.
type Component interface{}
