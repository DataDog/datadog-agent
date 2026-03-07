// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"time"
)

// OwnerKey exports ownerKey for testing.
type OwnerKey = ownerKey

// RolloutFunc is a function type implementing rollout for testing.
type RolloutFunc func(context.Context, OwnerKey, time.Time) (bool, error)

func (f RolloutFunc) restart(ctx context.Context, k ownerKey, ts time.Time) (bool, error) {
	return f(ctx, k, ts)
}

var _ rollout = RolloutFunc(nil)

// NewSchedulerForTest exports newScheduler for testing.
var NewSchedulerForTest = newScheduler
