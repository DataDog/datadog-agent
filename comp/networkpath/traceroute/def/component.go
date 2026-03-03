// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package traceroute provides the traceroute component
package traceroute

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
)

// team: network-path

// Component is the component type.
type Component interface {
	Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error)
}
