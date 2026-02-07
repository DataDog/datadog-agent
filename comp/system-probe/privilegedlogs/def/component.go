// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package privilegedlogs ... /* TODO: detailed doc comment for the component */
package privilegedlogs

import "github.com/DataDog/datadog-agent/comp/system-probe/types"

// team: agent-discovery agent-log-pipelines

// Component is the component type.
type Component interface {
	types.SystemProbeModuleComponent
}
