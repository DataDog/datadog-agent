// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"time"
)

// Clock is injectable for tests. The command always installs WallClock; staging
// has no accelerated clock option.
type Clock interface {
	Now() time.Time
	WaitUntil(context.Context, time.Time) error
}
type WallClock struct{}

func (WallClock) Now() time.Time { return time.Now() }
func (WallClock) WaitUntil(ctx context.Context, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delay := time.Until(at)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
