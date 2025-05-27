// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package daemonchecker ... /* TODO: detailed doc comment for the component */
package daemonchecker

import "github.com/DataDog/datadog-agent/pkg/fleet/daemon"

// team: fleet

// Component is the component type.
type Component interface {
	daemon.Checker
}
