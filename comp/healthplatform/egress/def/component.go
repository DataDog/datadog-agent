// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package egress defines the interface for the health platform egress component.
package egress

// team: fleet-remediation

// Component is the health platform egress component interface.
// Egress drives the periodic outbound HTTP POST to the Datadog intake:
// on each tick it calls store.GetAllIssues(), builds a HealthReport, and
// forwards it via forwarder.Send. It has no public methods — behaviour is
// driven entirely by its fx lifecycle hooks.
type Component interface{}
