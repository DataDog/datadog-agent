// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package doqueryactions provides the DO query actions component
package doqueryactions

// team: data-observability

// Component is the DO query actions component interface.
// This component subscribes to RC DEBUG product to receive declarative query configs,
// each containing the full set of active monitor queries for a DB instance.
// It schedules long-lived checks that manage query scheduling internally.
// Gated by the data_observability.query_actions.enabled feature flag (default: false).
type Component interface{}
