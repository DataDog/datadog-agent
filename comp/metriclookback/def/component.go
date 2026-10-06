// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metriclookback defines the metric lookback component.
package metriclookback

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// team: q-branch

// Component is the metric lookback component.
type Component interface {
	// NewSenderManager returns the sender manager used exclusively by metric
	// lookback shadow checks. It returns nil when lookback is unavailable in the
	// current Agent build.
	NewSenderManager(context.Context, string) sender.SenderManager

	// NewShadowCheckFactory creates optional shadow-check support for a scheduler.
	// Each scheduler owns its factory; calls are serialized by the scheduler.
	NewShadowCheckFactory(context.Context, string) ShadowCheckFactory
}

// ShadowCheckFactory prepares loaders for selected instances without exposing
// metric lookback policy, retention, or foreign-runtime routing to the scheduler.
// Implementations need not support concurrent calls.
type ShadowCheckFactory interface {
	Prepare(integration.Config) map[int]ShadowCheckLoader
}

// ShadowCheckLoader loads a shadow of a successfully loaded source check.
// Returning nil, nil means that the source loader does not support shadows.
// The returned check owns its sender-manager override and its cleanup.
type ShadowCheckLoader func(check.Loader, checkid.ID) (check.Check, error)
