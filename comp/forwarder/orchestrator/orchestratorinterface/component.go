// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package orchestratorinterface defines the interface for the orchestrator forwarder component.
package orchestratorinterface

import "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"

// team: agent-metrics-logs

// Component is the component type.
// The main method of this component is `Get` which returns the forwarder instance only if it enabled.
type Component interface {
	// Get the forwarder instance if it exists.
	Get() (defaultforwarder.Forwarder, bool)
}
