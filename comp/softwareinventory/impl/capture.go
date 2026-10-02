// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package softwareinventoryimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// captureReady advertises a successful running collector and an available output.
// GetCheck may return after cancellation. Readiness and shutdown share a lock so
// that late collection completion cannot resurrect a stopped producer.
func (is *softwareInventory) captureReady(ctx context.Context) {
	if is.captureManager == nil {
		return
	}
	is.captureMu.Lock()
	defer is.captureMu.Unlock()
	if is.captureStopped || !is.enabled || ctx.Err() != nil {
		return
	}
	if _, ok := is.eventPlatform.Get(); ok {
		_ = is.captureManager.Register(telemetrycapture.Capability{Stream: telemetrycapture.Software, Cadence: is.interval})
	} else {
		is.captureManager.Unregister(telemetrycapture.Software)
	}
}

func (is *softwareInventory) captureUnavailable() {
	if is.captureManager == nil {
		return
	}
	is.captureMu.Lock()
	defer is.captureMu.Unlock()
	is.captureManager.Unregister(telemetrycapture.Software)
}

func (is *softwareInventory) stopCapture() {
	if is.captureManager == nil {
		return
	}
	is.captureMu.Lock()
	defer is.captureMu.Unlock()
	is.captureStopped = true
	is.captureManager.Unregister(telemetrycapture.Software)
}
