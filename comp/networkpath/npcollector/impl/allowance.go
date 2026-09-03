// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npcollectorimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

const standardAllowancePerHour = 5

func (s *npCollectorImpl) inAllowance(profile payload.DynamicTestProfile) bool {
	switch profile {
	case payload.DynamicTestProfileBasic:
		return true
	case payload.DynamicTestProfileStandard:
		return s.takeStandardAllowance(s.TimeNowFn())
	default:
		return false
	}
}

// takeStandardAllowance returns true for the first standardAllowancePerHour
// completed standard runs in each hour. The hour starts on the first take.
func (s *npCollectorImpl) takeStandardAllowance(now time.Time) bool {
	s.allowanceMu.Lock()
	defer s.allowanceMu.Unlock()
	if s.allowanceUntil.IsZero() || !now.Before(s.allowanceUntil) {
		s.allowanceUntil = now.Add(time.Hour)
		s.allowanceLeft = standardAllowancePerHour
	}
	if s.allowanceLeft == 0 {
		return false
	}
	s.allowanceLeft--
	return true
}
