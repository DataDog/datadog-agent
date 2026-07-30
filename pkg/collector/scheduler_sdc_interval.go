// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender/sdcsender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// sdcIntervalOverrideCheck wraps a check.Check to override its scheduling
// interval — generalizes the gpu check's own hardcoded
// gpu.collection_interval_override pattern (pkg/collector/corechecks/gpu)
// to any SDC-compressed check, via checks.sdc_compression_interval_override.
type sdcIntervalOverrideCheck struct {
	check.Check
	interval time.Duration
}

func (c *sdcIntervalOverrideCheck) Interval() time.Duration {
	return c.interval
}

// wrapWithSDCIntervalOverride wraps ch to override its scheduling interval
// when checkName is SDC-compressed (per sdcsender.CompressionEnabledFor) and
// checks.sdc_compression_interval_override is set to a positive number of
// seconds, taking precedence over the check's own min_collection_interval.
// Otherwise ch is returned unchanged.
//
// A check whose own Interval() is already 0 is left untouched even when
// otherwise eligible: 0 is not "unset" here, it's a distinct scheduling mode
// (pkg/collector/scheduler/scheduler.go's Scheduler.Enter routes it to
// enqueueOnce instead of the normal ticker) used by long-running checks
// (e.g. container_image, sbom) that manage their own lifecycle. Forcing a
// positive interval onto one of those would incorrectly convert it into a
// normal ticked check.
func wrapWithSDCIntervalOverride(ch check.Check, checkName string) check.Check {
	iv := setup.Datadog().GetInt("checks.sdc_compression_interval_override")
	if iv <= 0 || ch.Interval() == 0 || !sdcsender.CompressionEnabledFor(checkName) {
		return ch
	}
	return &sdcIntervalOverrideCheck{Check: ch, interval: time.Duration(iv) * time.Second}
}
