// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npcollectorimpl

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

const (
	standardAllowancePerHour = 5
	standardAllowanceWindow  = time.Hour
)

type allowance struct {
	mu    sync.Mutex
	until time.Time
	left  int
}

func newAllowance() *allowance {
	return &allowance{}
}

func (a *allowance) inAllowance(profile payload.DynamicTestProfile, now time.Time) bool {
	switch profile {
	case payload.DynamicTestProfileBasic:
		return true
	case payload.DynamicTestProfileStandard:
		return a.take(now)
	default:
		return false
	}
}

// take returns true for the first standardAllowancePerHour completed
// standard runs in each hour. The hour starts on the first take.
func (a *allowance) take(now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.until.IsZero() || !now.Before(a.until) {
		a.until = now.Add(standardAllowanceWindow)
		a.left = standardAllowancePerHour
	}
	if a.left == 0 {
		return false
	}
	a.left--
	return true
}
