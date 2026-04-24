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
// It injects data_observability config into matching postgres check instances.
// Activates when a postgres instance with data_observability.enabled: true is detected.
type Component interface{}
