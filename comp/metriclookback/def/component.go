// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metriclookback defines the metric lookback component.
package metriclookback

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// team: q-branch

// Component is the metric lookback component.
type Component interface {
	// NewSenderManager returns the sender manager used exclusively by metric
	// lookback shadow checks. It returns nil when lookback is unavailable in the
	// current Agent build.
	NewSenderManager(context.Context, string) sender.SenderManager
}
