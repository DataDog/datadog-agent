// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteflags provides the Remote Flags component for dynamic feature flag management.
package remoteflags

import (
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
)

// team: agent-configuration

// Component is the Remote Flags component interface.
type Component interface {
	// GetClient returns the remote flags client for subscribing to feature flags.
	GetClient() *remoteflags.Client
}
