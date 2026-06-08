// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// shadowCheck wraps a check.Check and overrides ID() to append a ":shadow" suffix.
// This prevents collisions with the normal check in the per-process expvars globals
// (runningChecksStats, checkStats) and in the dedicated shadow runner's checksTracker.
// Every other method delegates transparently to the embedded check.
type shadowCheck struct {
	check.Check
}

func newShadowCheck(c check.Check) *shadowCheck {
	return &shadowCheck{c}
}

// ID returns the underlying check ID with ":shadow" appended.
func (s *shadowCheck) ID() checkid.ID {
	return checkid.ID(string(s.Check.ID()) + ":shadow")
}
